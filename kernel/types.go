// kernel/types.go

package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"pegasus_suite/clients"
	"pegasus_suite/engine"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrNotRunning     = errors.New("runtime not running")
	ErrAlreadyRunning = errors.New("runtime already running")
	ErrUnknownApp     = errors.New("unknown application")
	ErrAppDown        = errors.New("application not running")
	ErrNotFound       = errors.New("process not found")
	ErrNoSettings     = errors.New("no settings document for process")
	ErrExists         = errors.New("process already added")

	ErrInvalidSettings = errors.New("invalid settings")
	ErrUnknownSettings = errors.New("unknown settings document")
	ErrNotReloaded     = errors.New("settings saved; process did not reload")
)

// Kernel hands this down to an application
type Host interface {
	// returns admin-level settings document by name
	Settings(name string) (json.RawMessage, error)

	// betting engine
	Engine() *engine.Engine
}

// application: information -> logic -> bet signal
type App interface {
	Name() string
	Start(ctx context.Context, h Host) error
	Stop()
	Status() error
	NewProcess(key clients.ProcessKey, settings json.RawMessage, h Host) (Process, error)

	// returns a new value of the process settings type
	ProcessSettings() Settings

	// admin settings per application
	AdminSettings() map[string]func() Settings
}

// one running instance of an application for one user
// close releases betting sessions and removes settings
type Process interface {
	Start()
	Stop()
	Running() bool
	Close()
}

type AppStatus struct {
	Running bool   `json:"running"`
	Error   string `json:"error,omitempty"`
	Detail  any    `json:"detail,omitempty"`
}

type Status struct {
	Running   bool                 `json:"running"`
	StartedAt *time.Time           `json:"started_at,omitempty"`
	LastError string               `json:"last_error,omitempty"`
	Apps      map[string]AppStatus `json:"apps"`
}

type ProcessInfo struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type Kernel struct {
	store  clients.Store
	apps   []App
	byName map[string]App

	// every admin-level document name and type
	adminDocs map[string]func() Settings

	mu     sync.Mutex
	cancel context.CancelFunc
	eng    *engine.Engine
	procs  map[clients.ProcessKey]Process
	up     map[string]bool

	running   atomic.Bool
	startedAt atomic.Int64
	lastErr   atomic.Pointer[string]
	appErrs   atomic.Pointer[map[string]string]
}
