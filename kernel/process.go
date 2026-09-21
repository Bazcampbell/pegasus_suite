// kernel/process.go

package kernel

import (
	"errors"
	"fmt"
	"pegasus_suite/clients"
	"pegasus_suite/logger"
)

func (k *Kernel) AddProcess(key clients.ProcessKey) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if err := k.addLocked(key); err != nil {
		return err
	}
	k.setState(key, clients.StateStopped)
	return nil
}

func (k *Kernel) StartProcess(key clients.ProcessKey) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	p, err := k.getProcessByKey(key)
	if err != nil {
		return err
	}
	p.Start()
	k.setState(key, clients.StateRunning)

	logger.Info(logger.Log{Application: key.App, FormattedMessage: "started process", UserID: key.UserID, ProcessID: key.ProcessID})
	return nil
}

func (k *Kernel) StopProcess(key clients.ProcessKey) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	p, err := k.getProcessByKey(key)
	if err != nil {
		return err
	}
	p.Stop()
	k.setState(key, clients.StateStopped)

	logger.Debug(logger.Log{Application: key.App, FormattedMessage: "stopped process", UserID: key.UserID, ProcessID: key.ProcessID})
	return nil
}

// rebuilds a process from current settings, called after settings saved
func (k *Kernel) RestartProcess(key clients.ProcessKey) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	old, err := k.getProcessByKey(key)
	if err != nil {
		return err
	}
	if err := k.rebuildLocked(key, old); err != nil {
		return fmt.Errorf("process removed; settings did not load: %w", err)
	}

	k.procs[key].Start()
	k.setState(key, clients.StateRunning)

	logger.Debug(logger.Log{Application: key.App, FormattedMessage: "restarted process", UserID: key.UserID, ProcessID: key.ProcessID})
	return nil
}

func (k *Kernel) DeleteProcess(key clients.ProcessKey) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	p, err := k.getProcessByKey(key)
	if err != nil {
		return err
	}

	p.Stop()
	p.Close()
	delete(k.procs, key)
	k.eng.Release(key)

	if err := k.store.Forget(key); err != nil {
		logger.Warn(logger.Log{Application: key.App, FormattedMessage: fmt.Sprintf("unable to forget process state error=%v", err), UserID: key.UserID, ProcessID: key.ProcessID})
	}

	logger.Debug(logger.Log{Application: key.App, FormattedMessage: "deleted process", UserID: key.UserID, ProcessID: key.ProcessID})
	return nil
}

func (k *Kernel) ProcessRunning(key clients.ProcessKey) (bool, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	p, err := k.getProcessByKey(key)
	if err != nil {
		return false, err
	}
	return p.Running(), nil
}

// replaces a loaded process with one built from current settings, not started
func (k *Kernel) rebuildLocked(key clients.ProcessKey, old Process) error {
	old.Stop()
	old.Close()
	delete(k.procs, key)
	k.eng.Release(key)

	if err := k.addLocked(key); err != nil {
		k.setState(key, clients.StateStopped)
		return err
	}
	return nil
}

// builds and registers a process from its settings, not started
func (k *Kernel) addLocked(key clients.ProcessKey) error {
	app, err := k.getAppByProcessKey(key)
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
		// a session claimed before the failure must be released.
		k.eng.Release(key)
		return err
	}
	k.procs[key] = p

	logger.Debug(logger.Log{Application: key.App, FormattedMessage: "added process", UserID: key.UserID, ProcessID: key.ProcessID})
	return nil
}

// re-adds every process as per the store's state
func (k *Kernel) restoreProcessesLocked() {
	for _, app := range k.apps {
		if !k.up[app.Name()] {
			continue
		}

		refs, err := k.store.Processes(app.Name())
		if err != nil {
			logger.Warn(logger.Log{Application: app.Name(), FormattedMessage: fmt.Sprintf("%s: unable to list processes to restore error=%v", app.Name(), err)})
			continue
		}

		for _, ref := range refs {
			if err := k.addLocked(ref.Key); err != nil {
				logger.Warn(logger.Log{
					Application:      ref.Key.App,
					FormattedMessage: fmt.Sprintf("unable to restore process error=%v", err),
					UserID:           ref.Key.UserID,
					ProcessID:        ref.Key.ProcessID,
				})
				continue
			}
			if ref.State == clients.StateRunning {
				k.procs[ref.Key].Start()
			}
		}

		logger.Debug(logger.Log{Application: app.Name(), FormattedMessage: fmt.Sprintf("%s: restored processes count=%v", app.Name(), len(refs))})
	}
}

func (k *Kernel) getProcessByKey(key clients.ProcessKey) (Process, error) {
	if !k.running.Load() {
		return nil, ErrNotRunning
	}
	p, ok := k.procs[key]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}
