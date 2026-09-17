// clients/clients.go
//
// The client store: who the users are, which application processes they have,
// and the settings for each. Settings are JSON documents; the kernel is the
// only reader and hands each application its own document to decode.

package clients

import (
	"encoding/json"
	"errors"
)

var ErrNotFound = errors.New("clients: not found")

// ProcessKey identifies one running instance of one application for one user.
// It is the one identity used everywhere.
type ProcessKey struct {
	App       string
	UserID    string
	ProcessID string
}

func (k ProcessKey) String() string {
	return k.App + "/" + k.UserID + "/" + k.ProcessID
}

// State is what a process was last told to do, persisted so a boot restores it.
type State string

const (
	StateRunning State = "running"
	StateStopped State = "stopped"
)

type ProcessRef struct {
	Key   ProcessKey
	State State
}

type Store interface {
	// Process returns one process's settings document, or ErrNotFound.
	Process(key ProcessKey) (json.RawMessage, error)

	// App returns the admin-level document for a settings scope: an
	// application's own name for its feeds, or "betmatic"/"betfair" for the
	// admin accounts. An absent document reads as "{}".
	App(scope string) (json.RawMessage, error)

	// Processes lists every process recorded for an application with its last
	// state, so a fresh boot can restore what was running.
	Processes(application string) ([]ProcessRef, error)
	SetState(key ProcessKey, state State) error
	Forget(key ProcessKey) error
}
