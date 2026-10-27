package actor

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
