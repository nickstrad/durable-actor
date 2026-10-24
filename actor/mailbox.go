package actor

import "time"

// Scheduler runs actions at a later point in virtual time.
// sim.Scheduler implements it.
type Scheduler interface {
	Clock
	After(delay time.Duration, action func())
}

type Command interface {
	isCommand()
}

type PutCommand struct {
	Key   string
	Value string
	TTL   time.Duration
}

func (PutCommand) isCommand() {}

type GetCommand struct {
	Key   string
	Reply func(value string, found bool)
}

func (GetCommand) isCommand() {}

// MailboxActor processes commands through
// a scheduler-controlled FIFO mailbox.
type MailboxActor struct {
	sched         Scheduler
	state         *Actor
	inbox         []Command
	pumpScheduled bool
}

func NewMailboxActor(sched Scheduler) *MailboxActor {
	return &MailboxActor{
		sched: sched,
		state: NewActor(sched),
	}
}

// Send queues a command without executing it.
func (a *MailboxActor) Send(cmd Command) {
	a.inbox = append(a.inbox, cmd)

	if !a.pumpScheduled {
		a.pumpScheduled = true
		a.sched.After(0, a.processOne)
	}
}

// processOne handles exactly one command.
func (a *MailboxActor) processOne() {
	cmd := a.inbox[0]
	a.inbox = a.inbox[1:]

	switch c := cmd.(type) {
	case PutCommand:
		a.state.Put(c.Key, c.Value, c.TTL)

	case GetCommand:
		value, found := a.state.Get(c.Key)
		if c.Reply != nil {
			c.Reply(value, found)
		}

	default:
		panic("unsupported command")
	}

	// Schedule the next command, if any.
	if len(a.inbox) > 0 {
		a.sched.After(0, a.processOne)
	} else {
		a.pumpScheduled = false
	}
}
