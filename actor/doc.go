// Package actor implements actors that run inside a deterministic
// simulation: a key/value actor with expiring keys, a FIFO mailbox
// that runs one command per scheduler event, a simulated network
// between mailboxes, and a sandbox controller and worker that talk
// over that network with retries, deadlines, and idempotent starts.
//
// All time and concurrency come from an injected Scheduler, so the
// same inputs always produce the same sequence of events.
package actor
