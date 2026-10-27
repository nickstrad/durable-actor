package actor

import "time"

// StartStatus is the controller's view of one start request.
type StartStatus string

const (
	StartPending      StartStatus = "pending"
	StartAcknowledged StartStatus = "acknowledged"
	StartTimedOut     StartStatus = "timed_out"
	StartRejected     StartStatus = "rejected"
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

	r := &startRequest{key: key, status: StartPending}
	c.requests[key] = r

	// The deadline is its own event. It is scheduled before any reply
	// can arrive, so at the same instant it runs first.
	c.sched.After(c.timeout, func() {
		if r.status == StartPending {
			r.status = StartTimedOut
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
		if r.status == StartPending {
			c.send(r)
		}
	})
}

func (c *SandboxController) receive(cmd Command) {
	var key requestKey
	var status StartStatus

	switch m := cmd.(type) {
	case StartAck:
		key, status = requestKey{m.RequestID, m.SandboxID}, StartAcknowledged

	case StartConflict:
		key, status = requestKey{m.RequestID, m.SandboxID}, StartRejected

	default:
		panic("unsupported command")
	}

	// Ignore unknown and late replies.
	r, ok := c.requests[key]
	if !ok || r.status != StartPending {
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
