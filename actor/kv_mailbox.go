package actor

import "time"

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

// NewMailboxActor returns a mailbox in front of a TTL key/value Actor.
func NewMailboxActor(sched Scheduler) *MailboxActor {
	state := NewActor(sched)

	return NewMailbox(sched, func(cmd Command) {
		switch c := cmd.(type) {
		case PutCommand:
			state.Put(c.Key, c.Value, c.TTL)

		case GetCommand:
			value, found := state.Get(c.Key)
			if c.Reply != nil {
				c.Reply(value, found)
			}

		default:
			panic("unsupported command")
		}
	})
}
