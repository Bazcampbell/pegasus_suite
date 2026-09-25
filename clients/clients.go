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

	// AppSettings returns an admin-level settings document by name: a feed
	// such as "triples", or a shared account such as "betfair". An absent
	// document reads as "{}".
	AppSettings(name string) (json.RawMessage, error)

	// PutProcess and PutAppSettings replace a settings document whole. The
	// store does not validate; the kernel has by the time these are called.
	PutProcess(key ProcessKey, doc json.RawMessage) error
	PutAppSettings(name string, doc json.RawMessage) error

	// ProcessIDs lists the processes a user has settings documents for in
	// one application. DeleteProcess removes one; absent is not an error.
	ProcessIDs(application, userID string) ([]string, error)
	DeleteProcess(key ProcessKey) error

	// Processes lists every process recorded for an application with its last
	// state, so a fresh boot can restore what was running.
	Processes(application string) ([]ProcessRef, error)
	SetState(key ProcessKey, state State) error
	Forget(key ProcessKey) error

	// Runtime is what the whole runtime was last told to do, so a boot can resume it.
	Runtime() (State, error)
	SetRuntime(state State) error
}
