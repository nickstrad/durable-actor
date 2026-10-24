package sim

import (
	"container/heap"
	"time"
)

type event struct {
	at     time.Time
	order  uint64
	action func()
}

type eventQueue []event

func (q eventQueue) Len() int {
	return len(q)
}

func (q eventQueue) Less(i, j int) bool {
	if c := q[i].at.Compare(q[j].at); c != 0 {
		return c < 0
	}

	return q[i].order < q[j].order
}

func (q eventQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

func (q *eventQueue) Push(x any) {
	*q = append(*q, x.(event))
}

func (q *eventQueue) Pop() any {
	n := len(*q)
	e := (*q)[n-1]
	(*q)[n-1] = event{} // drop the closure so it can be collected
	*q = (*q)[:n-1]
	return e
}

type Scheduler struct {
	now       time.Time
	nextOrder uint64
	events    eventQueue
}

func NewScheduler(start time.Time) *Scheduler {
	return &Scheduler{now: start}
}

func (s *Scheduler) Now() time.Time {
	return s.now
}

// After schedules an event relative to virtual time.
func (s *Scheduler) After(delay time.Duration, action func()) {
	if delay < 0 {
		panic("negative delay")
	}

	if action == nil {
		panic("nil action")
	}

	heap.Push(&s.events, event{
		at:     s.now.Add(delay),
		order:  s.nextOrder,
		action: action,
	})

	s.nextOrder++
}

// RunNext executes one event, advancing virtual time.
func (s *Scheduler) RunNext() bool {
	if len(s.events) == 0 {
		return false
	}

	e := heap.Pop(&s.events).(event)

	s.now = e.at
	e.action()

	return true
}

// RunUntil executes events due by deadline,
// including events scheduled by callbacks.
// It then advances virtual time to deadline.
func (s *Scheduler) RunUntil(deadline time.Time) int {
	if deadline.Before(s.now) {
		panic("deadline in the past")
	}

	executed := 0

	for len(s.events) > 0 && !s.events[0].at.After(deadline) {
		s.RunNext()
		executed++
	}

	s.now = deadline

	return executed
}
