package actor

import (
	"fmt"
	"math/rand"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nickstrad/durable-actor/sim"
)

// numKeys is the size of the key space the workload draws from.
const numKeys = 4

func keyName(i int) string {
	return fmt.Sprintf("key-%d", i)
}

type modelEntry struct {
	value     string
	expiresAt time.Duration
}

// referenceModel is a deliberately simple, independent model of the
// actor's expected behavior. It keeps its own clock as an offset from
// the start of the run.
type referenceModel struct {
	now     time.Duration
	entries map[string]modelEntry
}

func newReferenceModel() *referenceModel {
	return &referenceModel{
		entries: make(map[string]modelEntry),
	}
}

func (m *referenceModel) put(key, value string, ttl time.Duration) {
	m.entries[key] = modelEntry{
		value:     value,
		expiresAt: m.now + ttl,
	}
}

func (m *referenceModel) get(key string) (string, bool) {
	e, exists := m.entries[key]

	if !exists || m.now >= e.expiresAt {
		return "", false
	}

	return e.value, true
}

type opKind int

const (
	opPut opKind = iota
	opGet
	opAdvance
)

func (k opKind) String() string {
	return [...]string{"put", "get", "advance"}[k]
}

type operation struct {
	kind         opKind
	key, value   string
	ttl, advance time.Duration
}

func generateOps(seed int64, count int) []operation {
	rng := rand.New(rand.NewSource(seed))
	ops := make([]operation, 0, count)

	for i := 0; i < count; i++ {
		key := keyName(rng.Intn(numKeys))

		switch rng.Intn(10) {
		case 0, 1, 2, 3:
			// 40% writes, including overwrites
			ops = append(ops, operation{
				kind:  opPut,
				key:   key,
				value: fmt.Sprintf("value-%d", i),
				ttl:   time.Duration(rng.Intn(20)+1) * time.Millisecond,
			})

		case 4, 5, 6, 7:
			// 40% reads
			ops = append(ops, operation{
				kind: opGet,
				key:  key,
			})

		default:
			// 20% virtual-time advances
			ops = append(ops, operation{
				kind:    opAdvance,
				advance: time.Duration(rng.Intn(25)+1) * time.Millisecond,
			})
		}
	}

	return ops
}

func runScenario(t *testing.T, seed int64) {
	t.Helper()

	sched := sim.NewScheduler(time.Unix(0, 0))
	actor := NewKVMailbox(sched)
	model := newReferenceModel()

	var trace []string

	// checkGet reads key through the mailbox and compares the reply
	// with the model.
	checkGet := func(label, key string) {
		t.Helper()

		var got string
		var found, replied bool

		actor.Send(GetCommand{
			Key: key,
			Reply: func(value string, ok bool) {
				got, found, replied = value, ok, true
			},
		})

		sched.RunUntil(sched.Now())

		want, exists := model.get(key)

		if !replied || got != want || found != exists {
			t.Fatalf(
				"seed=%d %s time=%s key=%s: "+
					"got (%q,%v,replied=%v), want (%q,%v)\n"+
					"trace:\n%s",
				seed, label, model.now, key,
				got, found, replied,
				want, exists,
				strings.Join(trace, "\n"),
			)
		}
	}

	for step, op := range generateOps(seed, 200) {
		trace = append(trace, fmt.Sprintf(
			"%03d t=%s %+v",
			step, model.now, op,
		))

		switch op.kind {
		case opPut:
			actor.Send(PutCommand{
				Key:   op.key,
				Value: op.value,
				TTL:   op.ttl,
			})

			sched.RunUntil(sched.Now())

			model.put(op.key, op.value, op.ttl)

		case opGet:
			checkGet(fmt.Sprintf("step=%d", step), op.key)

		case opAdvance:
			sched.RunUntil(sched.Now().Add(op.advance))
			model.now += op.advance
		}
	}

	// Observe every key after the generated workload.
	for i := 0; i < numKeys; i++ {
		checkGet("final", keyName(i))
	}
}

func TestWorkloadReproducible(t *testing.T) {
	a := generateOps(473829, 200)
	b := generateOps(473829, 200)

	if !slices.Equal(a, b) {
		t.Fatal("identical seed produced different operations")
	}
}

func TestActorRandomizedAgainstModel(t *testing.T) {
	// Replay one scenario if DST_SEED is provided.
	if raw := os.Getenv("DST_SEED"); raw != "" {
		seed, err := strconv.ParseInt(raw, 10, 64)

		if err != nil {
			t.Fatalf("invalid DST_SEED: %v", err)
		}

		runScenario(t, seed)
		return
	}

	for seed := int64(1); seed <= 100; seed++ {
		runScenario(t, seed)
	}
}
