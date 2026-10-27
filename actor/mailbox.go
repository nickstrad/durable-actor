package actor

// Command is a message an actor can receive. Only types in this
// package implement it.
type Command interface {
	isCommand()
}

// Mailbox processes commands through
// a scheduler-controlled FIFO mailbox.
type Mailbox struct {
	sched         Scheduler
	handle        func(Command)
	inbox         []Command
	pumpScheduled bool
}

// NewMailbox returns a mailbox that passes each command to handle,
// one at a time, in arrival order.
func NewMailbox(sched Scheduler, handle func(Command)) *Mailbox {
	if handle == nil {
		panic("nil handler")
	}

	return &Mailbox{
		sched:  sched,
		handle: handle,
	}
}

// Send queues a command without executing it.
func (a *Mailbox) Send(cmd Command) {
	a.inbox = append(a.inbox, cmd)

	if !a.pumpScheduled {
		a.pumpScheduled = true
		a.sched.After(0, a.processOne)
	}
}

// processOne handles exactly one command.
func (a *Mailbox) processOne() {
	cmd := a.inbox[0]
	a.inbox = a.inbox[1:]

	a.handle(cmd)

	// Schedule the next command, if any.
	if len(a.inbox) > 0 {
		a.sched.After(0, a.processOne)
	} else {
		a.pumpScheduled = false
	}
}
