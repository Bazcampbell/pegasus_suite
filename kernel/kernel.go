// kernel/kernel.go

// kernel hosts applications
// owns the restartable runtime, process registry, client store,
// betting session, API exposed controls

package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"pegasus_suite/clients"
	"pegasus_suite/engine"
	"pegasus_suite/logger"
)

func New(store clients.Store) *Kernel {
	k := &Kernel{
		store:     store,
		byName:    make(map[string]App),
		adminDocs: make(map[string]func() Settings),
		procs:     make(map[clients.ProcessKey]Process),
		up:        make(map[string]bool),
	}
	for name, newType := range shared {
		k.adminDocs[name] = newType
	}
	return k
}

// adds an application, called before start
func (k *Kernel) Register(app App) {
	if _, dup := k.byName[app.Name()]; dup {
		panic("kernel: application registered twice: " + app.Name())
	}
	for name, newType := range app.AdminSettings() {
		if _, dup := k.adminDocs[name]; dup {
			panic("kernel: settings document " + name + " claimed twice, again by " + app.Name())
		}
		k.adminDocs[name] = newType
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
			as.Detail = app.Status()
		}
		st.Apps[app.Name()] = as
	}
	return st
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

	// one application failing doesn't block the others
	// reported in status
	for _, app := range k.apps {
		if err := app.Start(ctx, h); err != nil {
			errs[app.Name()] = err.Error()
			logger.Error(logger.Log{
				App:     app.Name(),
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
		return k.fail(errors.New("no applications started"))
	}

	k.running.Store(true)
	k.startedAt.Store(time.Now().UnixNano())
	k.lastErr.Store(nil)

	k.restoreProcessesLocked()

	logger.Info(logger.Log{Message: fmt.Sprintf("runtime started apps=%v", k.Apps())})
	return nil
}

// stop serving, processes, applications
func (k *Kernel) stopLocked() error {
	if !k.running.Load() {
		return ErrNotRunning
	}

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

	logger.Info(logger.Log{Message: "runtime stopped"})
	return nil
}

func (k *Kernel) fail(err error) error {
	msg := err.Error()
	k.lastErr.Store(&msg)
	logger.Error(logger.Log{Message: fmt.Sprintf("runtime start failed error=%v", err)})
	return err
}

func (k *Kernel) getAppByProcessKey(key clients.ProcessKey) (App, error) {
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

func (k *Kernel) setState(key clients.ProcessKey, state clients.State) {
	if err := k.store.SetState(key, state); err != nil {
		logger.Warn(logger.Log{App: key.App, Message: fmt.Sprintf("unable to persist process state error=%v", err), UserID: key.UserID, ProcessID: key.ProcessID})
	}
}
