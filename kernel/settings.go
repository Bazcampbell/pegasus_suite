// kernel/settings.go
//
// user saves their own process settings, an admin can save anyones
// whole settings document is sent, the kernel decodes into
// the type its owner names and runs the type's Validate() before anything is
// written, so the store only ever holds documents that parse and are valid

package kernel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"pegasus_suite/clients"
	"pegasus_suite/engine"
	"pegasus_suite/logger"
)

type Settings interface {
	Validate() error
}

// admin-level, shared between any apps
var shared = map[string]func() Settings{
	"betfair":  func() Settings { return &engine.BetfairCredentials{} },
	"betmatic": func() Settings { return &engine.BetmaticCredentials{} },
}

func decode(doc json.RawMessage, into Settings) error {
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSettings, err)
	}
	if dec.More() {
		return fmt.Errorf("%w: trailing data after document", ErrInvalidSettings)
	}
	if err := into.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSettings, err)
	}
	return nil
}

func (k *Kernel) GetProcessSettings(key clients.ProcessKey) (json.RawMessage, error) {
	if _, ok := k.byName[key.App]; !ok {
		return nil, ErrUnknownApp
	}
	doc, err := k.store.Process(key)
	if errors.Is(err, clients.ErrNotFound) {
		return nil, ErrNoSettings
	}
	return doc, err
}

func (k *Kernel) SaveProcessSettings(key clients.ProcessKey, doc json.RawMessage) error {
	app, ok := k.byName[key.App]
	if !ok {
		return ErrUnknownApp
	}
	if err := decode(doc, app.ProcessSettings()); err != nil {
		return err
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	if err := k.store.PutProcess(key, doc); err != nil {
		return fmt.Errorf("unable to save process settings: %w", err)
	}
	logger.Info(logger.Log{Application: key.App, FormattedMessage: "saved process settings", UserID: key.UserID, ProcessID: key.ProcessID})

	if !k.running.Load() {
		return nil
	}
	old, loaded := k.procs[key]
	if !loaded {
		return nil
	}

	wasRunning := old.Running()
	if err := k.rebuildLocked(key, old); err != nil {
		return fmt.Errorf("%w: %v", ErrNotReloaded, err)
	}
	if wasRunning {
		k.procs[key].Start()
		k.setState(key, clients.StateRunning)
	}

	logger.Debug(logger.Log{Application: key.App, FormattedMessage: "reloaded process from saved settings", UserID: key.UserID, ProcessID: key.ProcessID})
	return nil
}

const (
	StatusActive   = "active"    // loaded and running
	StatusStopped  = "stopped"   // loaded, not running
	StatusNotAdded = "not-added" // settings saved, never added (or failed to load)
	StatusOffline  = "offline"   // the runtime or the application is down
)

// ListProcesses is every process a user has settings for in an application,
// sorted by id, with its status in the runtime
func (k *Kernel) ListProcesses(app, userID string) ([]ProcessInfo, error) {
	if _, ok := k.byName[app]; !ok {
		return nil, ErrUnknownApp
	}
	ids, err := k.store.ProcessIDs(app, userID)
	if err != nil {
		return nil, fmt.Errorf("unable to list processes: %w", err)
	}
	slices.Sort(ids)

	k.mu.Lock()
	defer k.mu.Unlock()

	up := k.running.Load() && k.up[app]
	out := make([]ProcessInfo, 0, len(ids))
	for _, id := range ids {
		info := ProcessInfo{ID: id, Status: StatusOffline}
		if up {
			info.Status = StatusNotAdded
			if p, ok := k.procs[clients.ProcessKey{App: app, UserID: userID, ProcessID: id}]; ok {
				info.Status = StatusStopped
				if p.Running() {
					info.Status = StatusActive
				}
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// removes a process + state + settings
func (k *Kernel) DeleteProcessSettings(key clients.ProcessKey) error {
	if _, ok := k.byName[key.App]; !ok {
		return ErrUnknownApp
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	if p, ok := k.procs[key]; ok && k.running.Load() {
		p.Stop()
		p.Close()
		delete(k.procs, key)
		k.eng.Release(key)
	}
	if err := k.store.Forget(key); err != nil {
		return fmt.Errorf("unable to forget process state: %w", err)
	}
	if err := k.store.DeleteProcess(key); err != nil {
		return fmt.Errorf("unable to delete process settings: %w", err)
	}

	logger.Debug(logger.Log{Application: key.App, FormattedMessage: "deleted process and its settings", UserID: key.UserID, ProcessID: key.ProcessID})
	return nil
}

func (k *Kernel) appSettingsType(name string) (Settings, bool) {
	newType, ok := k.adminDocs[name]
	if !ok {
		return nil, false
	}
	return newType(), true
}

// returns admin-level application settings
func (k *Kernel) AppSettings(name string) (json.RawMessage, error) {
	into, ok := k.appSettingsType(name)
	if !ok {
		return nil, ErrUnknownSettings
	}
	doc, err := k.store.AppSettings(name)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(doc, into); err != nil {
		return nil, fmt.Errorf("stored %s settings do not parse: %w", name, err)
	}
	return json.Marshal(into)
}

// validates and writes admin-level app settings
// changes apply on runtime restart
func (k *Kernel) SaveAppSettings(name string, doc json.RawMessage) error {
	into, ok := k.appSettingsType(name)
	if !ok {
		return ErrUnknownSettings
	}
	if err := decode(doc, into); err != nil {
		return err
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	if err := k.store.PutAppSettings(name, doc); err != nil {
		return fmt.Errorf("unable to save %s settings: %w", name, err)
	}
	logger.Debug(logger.Log{FormattedMessage: fmt.Sprintf("saved %s settings; applies on the next runtime restart", name)})
	return nil
}
