// kernel/kernel.go
//
// The kernel hosts applications. It owns what every application used to
// duplicate: the restartable runtime, the process registry, the client store,
// the betting sessions, and the HTTP control plane. It knows nothing about
// feeds, strategies, or what a process does with a message — an application
// owns all of that and the kernel only drives its lifecycle.

package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"racing_wagering/clients"
	"racing_wagering/engine"
	"racing_wagering/logger"
)

var (
	ErrNotRunning     = errors.New("runtime not running")
	ErrAlreadyRunning = errors.New("runtime already running")
	ErrUnknownApp     = errors.New("unknown application")
	ErrAppDown        = errors.New("application not running")
	ErrNotFound       = errors.New("process not found")
	ErrNoSettings     = errors.New("no settings document for process")
	ErrExists         = errors.New("process already added")
)

// Host is what the kernel hands down to an application. Everything an
// application needs from outside itself comes through here.
type Host interface {
	// App returns the admin-level settings document for a scope: the
	// application's own name for its feeds, or "betmatic"/"betfair" for the
	// admin accounts. Absent reads as "{}".
	App(scope string) (json.RawMessage, error)

	// Engine is the betting engine: sessions, staking, placement, results.
	Engine() *engine.Engine
}

// App is an application: a source of signal plus the logic that turns it into
// bets. Start brings up whatever it shares across processes (feeds, admin
// clients); NewProcess builds one user's instance from their settings document.
type App interface {
	Name() string
	Start(ctx context.Context, h Host) error
	Stop()
	NewProcess(key clients.ProcessKey, settings json.RawMessage, h Host) (Process, error)
}

// StatusReporter is optional. Its result is surfaced in Status under the app.
type StatusReporter interface {
	Status() any
}

// Process is one running instance of an application for one user. Start and
// Stop may be called repeatedly; Close is called once, when the process is
// removed, and after it the kernel releases the process's betting sessions.
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

type Kernel struct {
	store  clients.Store
	apps   []App
	byName map[string]App

	// mu serialises lifecycle and process operations.
	mu     sync.Mutex
	cancel context.CancelFunc
	eng    *engine.Engine
	procs  map[clients.ProcessKey]Process
	up     map[string]bool

	// read lock-free by Status, so a poll never waits behind a slow Start
	running   atomic.Bool
	startedAt atomic.Int64
	lastErr   atomic.Pointer[string]
	appErrs   atomic.Pointer[map[string]string]
}

func New(store clients.Store) *Kernel {
	return &Kernel{
		store:  store,
		byName: make(map[string]App),
		procs:  make(map[clients.ProcessKey]Process),
		up:     make(map[string]bool),
	}
}

// Register adds an application. Call before Start; order is start order.
func (k *Kernel) Register(app App) {
	if _, dup := k.byName[app.Name()]; dup {
		panic("kernel: application registered twice: " + app.Name())
	}
	k.apps = append(k.apps, app)
	k.byName[app.Name()] = app
}

func (k *Kernel) Apps() []string {
	names := make([]string, 0, len(k.apps))
	for _, a := range k.apps {
		names = append(names, a.Name())
	}
	return names
}

func (k *Kernel) HasApp(name string) bool {
	_, ok := k.byName[name]
	return ok
}

// ---- lifecycle ----

func (k *Kernel) Start() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.startLocked()
}

func (k *Kernel) Stop() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.stopLocked()
}

func (k *Kernel) Restart() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.running.Load() {
		if err := k.stopLocked(); err != nil {
			return err
		}
	}
	return k.startLocked()
}

func (k *Kernel) startLocked() error {
	if k.running.Load() {
		return ErrAlreadyRunning
	}

	ctx, cancel := context.WithCancel(context.Background())
	k.cancel = cancel
	k.eng = engine.New(ctx)

	h := &host{k: k}
	errs := make(map[string]string)

	// One application failing is not a reason to deny the others: it is
	// reported in Status and comes back on the next restart.
	for _, app := range k.apps {
		if err := app.Start(ctx, h); err != nil {
			errs[app.Name()] = err.Error()
			logger.Error(logger.ErrorLog{
				Message: fmt.Sprintf("%s did not start; nothing will run on it until a restart error=%v", app.Name(), err),
			})
			continue
		}
		k.up[app.Name()] = true
	}
	k.appErrs.Store(&errs)

	if len(k.up) == 0 {
		cancel()
		k.eng.Close()
		return k.fail(errors.New("no application started"))
	}

	k.running.Store(true)
	k.startedAt.Store(time.Now().UnixNano())
	k.lastErr.Store(nil)

	k.restoreLocked()

	logger.Info(logger.InfoLog{Message: fmt.Sprintf("runtime started apps=%v", k.Apps())})
	return nil
}

// restoreLocked re-adds every process the store remembers and starts the ones
// that were running. This is what makes a container restart pick up where it
// left off.
func (k *Kernel) restoreLocked() {
	for _, app := range k.apps {
		if !k.up[app.Name()] {
			continue
		}

		refs, err := k.store.Processes(app.Name())
		if err != nil {
			logger.Warn(logger.ErrorLog{Message: fmt.Sprintf("%s: unable to list processes to restore error=%v", app.Name(), err)})
			continue
		}

		for _, ref := range refs {
			if err := k.addLocked(ref.Key); err != nil {
				logger.Warn(logger.ErrorLog{
					Message:   fmt.Sprintf("unable to restore process error=%v", err),
					UserID:    ref.Key.UserID,
					ProcessID: ref.Key.ProcessID,
				})
				continue
			}
			if ref.State == clients.StateRunning {
				k.procs[ref.Key].Start()
			}
		}

		logger.Debug(logger.InfoLog{Message: fmt.Sprintf("%s: restored processes count=%v", app.Name(), len(refs))})
	}
}

func (k *Kernel) stopLocked() error {
	if !k.running.Load() {
		return ErrNotRunning
	}

	// Stop serving first, then tear down. Store state is left alone so the next
	// Start restores exactly this set.
	k.running.Store(false)

	for key, p := range k.procs {
		p.Stop()
		p.Close()
		delete(k.procs, key)
	}

	for _, app := range k.apps {
		if k.up[app.Name()] {
			app.Stop()
			delete(k.up, app.Name())
		}
	}

	k.eng.Close()
	k.cancel()
	k.cancel = nil

	logger.Info(logger.InfoLog{Message: "runtime stopped"})
	return nil
}

func (k *Kernel) fail(err error) error {
	msg := err.Error()
	k.lastErr.Store(&msg)
	logger.Error(logger.ErrorLog{Message: fmt.Sprintf("runtime start failed error=%v", err)})
	return err
}

func (k *Kernel) Status() Status {
	st := Status{Running: k.running.Load(), Apps: make(map[string]AppStatus, len(k.apps))}
	if st.Running {
		t := time.Unix(0, k.startedAt.Load())
		st.StartedAt = &t
	}
	if p := k.lastErr.Load(); p != nil {
		st.LastError = *p
	}

	var errs map[string]string
	if p := k.appErrs.Load(); p != nil {
		errs = *p
	}

	for _, app := range k.apps {
		as := AppStatus{Error: errs[app.Name()]}
		as.Running = st.Running && as.Error == ""
		if as.Running {
			if r, ok := app.(StatusReporter); ok {
				as.Detail = r.Status()
			}
		}
		st.Apps[app.Name()] = as
	}
	return st
}

// ---- processes ----

func (k *Kernel) AddProcess(key clients.ProcessKey) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if err := k.addLocked(key); err != nil {
		return err
	}
	k.setState(key, clients.StateStopped)
	return nil
}

func (k *Kernel) addLocked(key clients.ProcessKey) error {
	app, err := k.appFor(key)
	if err != nil {
		return err
	}
	if _, exists := k.procs[key]; exists {
		return ErrExists
	}

	doc, err := k.store.Process(key)
	if err != nil {
		if errors.Is(err, clients.ErrNotFound) {
			return ErrNoSettings
		}
		return fmt.Errorf("unable to load process settings: %w", err)
	}

	p, err := app.NewProcess(key, doc, &host{k: k})
	if err != nil {
		// A session claimed before the failure must not stay held.
		k.eng.Release(key)
		return err
	}
	k.procs[key] = p

	logger.Debug(logger.InfoLog{Message: "added process", UserID: key.UserID, ProcessID: key.ProcessID})
	return nil
}

func (k *Kernel) StartProcess(key clients.ProcessKey) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	p, err := k.processLocked(key)
	if err != nil {
		return err
	}
	p.Start()
	k.setState(key, clients.StateRunning)

	logger.Info(logger.InfoLog{Message: "started process", UserID: key.UserID, ProcessID: key.ProcessID})
	return nil
}

func (k *Kernel) StopProcess(key clients.ProcessKey) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	p, err := k.processLocked(key)
	if err != nil {
		return err
	}
	p.Stop()
	k.setState(key, clients.StateStopped)

	logger.Debug(logger.InfoLog{Message: "stopped process", UserID: key.UserID, ProcessID: key.ProcessID})
	return nil
}

// RestartProcess rebuilds a process from its current settings. Called after
// settings are saved.
func (k *Kernel) RestartProcess(key clients.ProcessKey) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	old, err := k.processLocked(key)
	if err != nil {
		return err
	}

	old.Stop()
	old.Close()
	delete(k.procs, key)
	k.eng.Release(key)

	if err := k.addLocked(key); err != nil {
		k.setState(key, clients.StateStopped)
		return fmt.Errorf("process removed; settings did not load: %w", err)
	}

	k.procs[key].Start()
	k.setState(key, clients.StateRunning)

	logger.Debug(logger.InfoLog{Message: "restarted process", UserID: key.UserID, ProcessID: key.ProcessID})
	return nil
}

func (k *Kernel) DeleteProcess(key clients.ProcessKey) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	p, err := k.processLocked(key)
	if err != nil {
		return err
	}

	p.Stop()
	p.Close()
	delete(k.procs, key)
	k.eng.Release(key)

	if err := k.store.Forget(key); err != nil {
		logger.Warn(logger.ErrorLog{Message: fmt.Sprintf("unable to forget process state error=%v", err), UserID: key.UserID, ProcessID: key.ProcessID})
	}

	logger.Debug(logger.InfoLog{Message: "deleted process", UserID: key.UserID, ProcessID: key.ProcessID})
	return nil
}

func (k *Kernel) ProcessRunning(key clients.ProcessKey) (bool, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	p, err := k.processLocked(key)
	if err != nil {
		return false, err
	}
	return p.Running(), nil
}

func (k *Kernel) appFor(key clients.ProcessKey) (App, error) {
	if !k.running.Load() {
		return nil, ErrNotRunning
	}
	app, ok := k.byName[key.App]
	if !ok {
		return nil, ErrUnknownApp
	}
	if !k.up[key.App] {
		return nil, ErrAppDown
	}
	return app, nil
}

func (k *Kernel) processLocked(key clients.ProcessKey) (Process, error) {
	if !k.running.Load() {
		return nil, ErrNotRunning
	}
	p, ok := k.procs[key]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

// setState persists the last instruction for a process. A failure here costs
// the restore on the next boot, not the process itself, so it only warns.
func (k *Kernel) setState(key clients.ProcessKey, state clients.State) {
	if err := k.store.SetState(key, state); err != nil {
		logger.Warn(logger.ErrorLog{Message: fmt.Sprintf("unable to persist process state error=%v", err), UserID: key.UserID, ProcessID: key.ProcessID})
	}
}

// ---- host ----

type host struct{ k *Kernel }

func (h *host) App(scope string) (json.RawMessage, error) { return h.k.store.App(scope) }
func (h *host) Engine() *engine.Engine                    { return h.k.eng }
