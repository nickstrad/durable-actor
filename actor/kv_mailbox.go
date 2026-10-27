package actor

import "time"

// PutCommand stores Value under Key for TTL.
type PutCommand struct {
	Key   string
	Value string
	TTL   time.Duration
}

func (PutCommand) isCommand() {}

// GetCommand reads Key and passes the result to Reply, if set.
type GetCommand struct {
	Key   string
	Reply func(value string, found bool)
}

func (GetCommand) isCommand() {}

// NewKVMailbox returns a mailbox in front of a TTL key/value Actor.
func NewKVMailbox(sched Scheduler) *Mailbox {
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
