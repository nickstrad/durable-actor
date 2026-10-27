package actor

// SandboxWorker starts sandboxes on request. It remembers each
// request ID, so a retried request does not start a sandbox twice.
type SandboxWorker struct {
	net     *SimNetwork
	address string
	handled map[string]string // request ID -> sandbox ID
	starts  map[string]int    // sandbox ID -> start count
}

// NewSandboxWorker registers a worker on net under address.
func NewSandboxWorker(sched Scheduler, net *SimNetwork, address string) *SandboxWorker {
	w := &SandboxWorker{
		net:     net,
		address: address,
		handled: make(map[string]string),
		starts:  make(map[string]int),
	}

	net.Register(address, NewMailbox(sched, w.receive))

	return w
}

func (w *SandboxWorker) receive(cmd Command) {
	req, ok := cmd.(StartRequest)
	if !ok {
		panic("unsupported command")
	}

	var reply Command = StartAck{RequestID: req.RequestID, SandboxID: req.SandboxID}

	if previous, seen := w.handled[req.RequestID]; seen {
		// A retry. Reply again but do not start again.
		if previous != req.SandboxID {
			reply = StartConflict{RequestID: req.RequestID, SandboxID: req.SandboxID}
		}
	} else {
		w.handled[req.RequestID] = req.SandboxID
		w.starts[req.SandboxID]++ // stand-in for starting a sandbox
	}

	// The reply travels through the simulated network.
	w.net.Send(w.address, req.ReplyTo, reply)
}

// StartCount returns how many times the worker started sandboxID.
func (w *SandboxWorker) StartCount(sandboxID string) int {
	return w.starts[sandboxID]
}
