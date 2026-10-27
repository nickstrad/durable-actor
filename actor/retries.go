package actor

import "time"

// StartRequest asks a worker to start a sandbox. The worker replies
// to ReplyTo. RequestID makes retries of the same request idempotent.
type StartRequest struct {
	RequestID string
	SandboxID string
	ReplyTo   string
}

// StartAck says the worker has started the sandbox for RequestID.
type StartAck struct {
	RequestID string
	SandboxID string
}

// StartConflict says RequestID was already used for a different sandbox.
type StartConflict struct {
	RequestID string
	SandboxID string
}

func (StartRequest) isCommand()  {}
func (StartAck) isCommand()      {}
func (StartConflict) isCommand() {}

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

type StartStatus string

const (
	Pending      StartStatus = "pending"
	Acknowledged StartStatus = "acknowledged"
	TimedOut     StartStatus = "timed_out"
	Rejected     StartStatus = "rejected"
)

// requestKey identifies one start request. A request ID reused for a
// different sandbox is a separate request, which the worker rejects.
type requestKey struct {
	requestID string
	sandboxID string
}

type startRequest struct {
	key      requestKey
	status   StartStatus
	attempts int
}

// SandboxController asks a worker to start sandboxes. It resends a
// request every retryEvery until it gets a reply or timeout passes.
type SandboxController struct {
	sched      Scheduler
	net        *SimNetwork
	address    string
	worker     string
	retryEvery time.Duration
	timeout    time.Duration
	requests   map[requestKey]*startRequest
}

// NewSandboxController registers a controller on net under address
// that sends its requests to the actor named worker.
func NewSandboxController(
	sched Scheduler,
	net *SimNetwork,
	address, worker string,
	retryEvery, timeout time.Duration,
) *SandboxController {
	if retryEvery <= 0 || timeout <= 0 {
		panic("retry and timeout must be positive")
	}

	c := &SandboxController{
		sched:      sched,
		net:        net,
		address:    address,
		worker:     worker,
		retryEvery: retryEvery,
		timeout:    timeout,
		requests:   make(map[requestKey]*startRequest),
	}

	net.Register(address, NewMailbox(sched, c.receive))

	return c
}

// Start sends a start request and schedules its deadline. Calling it
// again with the same IDs does nothing. Reusing requestID for a
// different sandbox sends a new request, which the worker rejects.
func (c *SandboxController) Start(requestID, sandboxID string) {
	if requestID == "" || sandboxID == "" {
		panic("empty ID")
	}

	key := requestKey{requestID, sandboxID}
	if _, exists := c.requests[key]; exists {
		return
	}

	r := &startRequest{key: key, status: Pending}
	c.requests[key] = r

	// The deadline is its own event. It is scheduled before any reply
	// can arrive, so at the same instant it runs first.
	c.sched.After(c.timeout, func() {
		if r.status == Pending {
			r.status = TimedOut
		}
	})

	c.send(r)
}

// send makes one attempt and schedules a retry while still pending.
func (c *SandboxController) send(r *startRequest) {
	r.attempts++

	c.net.Send(c.address, c.worker, StartRequest{
		RequestID: r.key.requestID,
		SandboxID: r.key.sandboxID,
		ReplyTo:   c.address,
	})

	c.sched.After(c.retryEvery, func() {
		if r.status == Pending {
			c.send(r)
		}
	})
}

func (c *SandboxController) receive(cmd Command) {
	var key requestKey
	var status StartStatus

	switch m := cmd.(type) {
	case StartAck:
		key, status = requestKey{m.RequestID, m.SandboxID}, Acknowledged

	case StartConflict:
		key, status = requestKey{m.RequestID, m.SandboxID}, Rejected

	default:
		panic("unsupported command")
	}

	// Ignore unknown and late replies.
	r, ok := c.requests[key]
	if !ok || r.status != Pending {
		return
	}

	r.status = status
}

// Result returns the status of a request and how many times it was sent.
func (c *SandboxController) Result(requestID, sandboxID string) (StartStatus, int) {
	r, ok := c.requests[requestKey{requestID, sandboxID}]
	if !ok {
		panic("unknown request")
	}

	return r.status, r.attempts
}
