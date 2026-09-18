package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"pegasus_suite/clients"
	"pegasus_suite/clients/mem"
)

// ---- a fake application ----

type fakeSettings struct {
	Stake float64 `json:"stake"`
}

func (s *fakeSettings) Validate() error {
	if s.Stake <= 0 {
		return errors.New("stake must be positive")
	}
	return nil
}

type fakeProcess struct {
	stake   float64
	running bool
}

func (p *fakeProcess) Start()        { p.running = true }
func (p *fakeProcess) Stop()         { p.running = false }
func (p *fakeProcess) Running() bool { return p.running }
func (p *fakeProcess) Close()        {}

type fakeApp struct{ built []*fakeProcess }

func (a *fakeApp) Name() string                      { return "fake" }
func (a *fakeApp) Start(context.Context, Host) error { return nil }
func (a *fakeApp) Stop()                             {}
func (a *fakeApp) ProcessSettings() Settings         { return &fakeSettings{} }
func (a *fakeApp) AdminSettings() map[string]func() Settings {
	return map[string]func() Settings{"fakefeed": func() Settings { return &fakeSettings{Stake: 1} }}
}
func (a *fakeApp) NewProcess(_ clients.ProcessKey, doc json.RawMessage, _ Host) (Process, error) {
	var s fakeSettings
	if err := json.Unmarshal(doc, &s); err != nil {
		return nil, err
	}
	p := &fakeProcess{stake: s.Stake}
	a.built = append(a.built, p)
	return p, nil
}

func newKernel(t *testing.T) (*Kernel, *mem.Store, *fakeApp) {
	t.Helper()
	store := mem.New()
	app := &fakeApp{}
	k := New(store)
	k.Register(app)
	return k, store, app
}

var key = clients.ProcessKey{App: "fake", UserID: "u1", ProcessID: "p1"}

// ---- process settings ----

func TestSaveProcessSettingsValidatesBeforeWriting(t *testing.T) {
	k, store, _ := newKernel(t)

	cases := map[string]string{
		"type's Validate fails": `{"stake": 0}`,
		"unknown field":         `{"stake": 5, "stak": 5}`,
		"not json":              `{"stake":`,
		"empty":                 ``,
		"trailing data":         `{"stake": 5} {}`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			err := k.SaveProcessSettings(key, json.RawMessage(doc))
			if !errors.Is(err, ErrInvalidSettings) {
				t.Fatalf("err = %v, want ErrInvalidSettings", err)
			}
			if _, err := store.Process(key); !errors.Is(err, clients.ErrNotFound) {
				t.Fatalf("invalid document was written")
			}
		})
	}
}

func TestSaveProcessSettingsWritesWhileRuntimeDown(t *testing.T) {
	k, store, _ := newKernel(t)

	doc := json.RawMessage(`{"stake": 5}`)
	if err := k.SaveProcessSettings(key, doc); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := store.Process(key)
	if err != nil || string(got) != string(doc) {
		t.Fatalf("stored = %s, %v; want %s", got, err, doc)
	}

	back, err := k.ProcessSettings(key)
	if err != nil || string(back) != string(doc) {
		t.Fatalf("ProcessSettings = %s, %v; want %s", back, err, doc)
	}
}

func TestSaveProcessSettingsUnknownApp(t *testing.T) {
	k, _, _ := newKernel(t)

	other := clients.ProcessKey{App: "nope", UserID: "u1", ProcessID: "p1"}
	if err := k.SaveProcessSettings(other, json.RawMessage(`{"stake": 5}`)); !errors.Is(err, ErrUnknownApp) {
		t.Fatalf("err = %v, want ErrUnknownApp", err)
	}
}

func TestSaveProcessSettingsReloadsLoadedProcess(t *testing.T) {
	k, _, app := newKernel(t)

	if err := k.SaveProcessSettings(key, json.RawMessage(`{"stake": 5}`)); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := k.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { k.Stop() })

	if err := k.AddProcess(key); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := k.StartProcess(key); err != nil {
		t.Fatalf("start process: %v", err)
	}

	if err := k.SaveProcessSettings(key, json.RawMessage(`{"stake": 9}`)); err != nil {
		t.Fatalf("save: %v", err)
	}

	latest := app.built[len(app.built)-1]
	if latest.stake != 9 {
		t.Fatalf("reloaded stake = %v, want 9", latest.stake)
	}
	if !latest.running {
		t.Fatalf("a running process should still be running after a save")
	}
}

// ---- admin-level settings ----

func TestSaveAppSettings(t *testing.T) {
	k, store, _ := newKernel(t)

	// never saved: reads back as the type's defaults, not "{}"
	if got, err := k.AppSettings("fakefeed"); err != nil || string(got) != `{"stake":1}` {
		t.Fatalf("unsaved document = %s, %v; want the defaults", got, err)
	}

	if err := k.SaveAppSettings("fakefeed", json.RawMessage(`{"stake": 2}`)); err != nil {
		t.Fatalf("app's own document: %v", err)
	}
	if got, _ := store.AppSettings("fakefeed"); string(got) != `{"stake": 2}` {
		t.Fatalf("stored = %s", got)
	}
	if err := k.SaveAppSettings("fakefeed", json.RawMessage(`{"stake": 0}`)); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("invalid app document: err = %v, want ErrInvalidSettings", err)
	}

	if err := k.SaveAppSettings("betfair", json.RawMessage(`{"username": "a"}`)); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("half-filled betfair: err = %v, want ErrInvalidSettings", err)
	}
	full := `{"username": "a", "password": "b", "app_key": "c", "cert": "d"}`
	if err := k.SaveAppSettings("betfair", json.RawMessage(full)); err != nil {
		t.Fatalf("betfair: %v", err)
	}

	if err := k.SaveAppSettings("nope", json.RawMessage(`{}`)); !errors.Is(err, ErrUnknownSettings) {
		t.Fatalf("unknown name: err = %v, want ErrUnknownSettings", err)
	}
	if _, err := k.AppSettings("nope"); !errors.Is(err, ErrUnknownSettings) {
		t.Fatalf("unknown name read: err = %v, want ErrUnknownSettings", err)
	}
}

func TestRegisterRefusesSecondOwner(t *testing.T) {
	k, _, _ := newKernel(t)

	defer func() {
		if recover() == nil {
			t.Fatalf("a second owner for a settings document should panic")
		}
	}()
	k.Register(&claimsBetfair{})
}

// claimsBetfair is an app that wrongly claims a shared document.
type claimsBetfair struct{ fakeApp }

func (a *claimsBetfair) Name() string { return "other" }
func (a *claimsBetfair) AdminSettings() map[string]func() Settings {
	return map[string]func() Settings{"betfair": func() Settings { return &fakeSettings{} }}
}

// ---- listing and deleting ----

func TestListAndDeleteProcesses(t *testing.T) {
	k, store, _ := newKernel(t)

	for _, pid := range []string{"b", "a"} {
		key := clients.ProcessKey{App: "fake", UserID: "u1", ProcessID: pid}
		if err := k.SaveProcessSettings(key, json.RawMessage(`{"stake": 5}`)); err != nil {
			t.Fatalf("save %s: %v", pid, err)
		}
	}
	other := clients.ProcessKey{App: "fake", UserID: "u2", ProcessID: "x"}
	if err := k.SaveProcessSettings(other, json.RawMessage(`{"stake": 5}`)); err != nil {
		t.Fatalf("save other user: %v", err)
	}

	// runtime down: everything is listed, all offline, only u1's
	list, err := k.ListProcesses("fake", "u1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].ID != "a" || list[1].ID != "b" || list[0].Status != StatusOffline {
		t.Fatalf("list = %+v; want a, b offline", list)
	}

	if err := k.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { k.Stop() })

	a := clients.ProcessKey{App: "fake", UserID: "u1", ProcessID: "a"}
	if err := k.AddProcess(a); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := k.StartProcess(a); err != nil {
		t.Fatalf("start process: %v", err)
	}

	list, _ = k.ListProcesses("fake", "u1")
	if list[0].Status != StatusActive || list[1].Status != StatusNotAdded {
		t.Fatalf("list = %+v; want a active, b not-added", list)
	}

	if err := k.DeleteProcessSettings(a); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Process(a); !errors.Is(err, clients.ErrNotFound) {
		t.Fatalf("settings document survived delete")
	}
	if _, err := k.ProcessRunning(a); !errors.Is(err, ErrNotFound) {
		t.Fatalf("process still loaded after delete: err = %v", err)
	}
	refs, _ := store.Processes("fake")
	for _, ref := range refs {
		if ref.Key == a {
			t.Fatalf("state survived delete, so a boot would try to restore it")
		}
	}

	list, _ = k.ListProcesses("fake", "u1")
	if len(list) != 1 || list[0].ID != "b" {
		t.Fatalf("list after delete = %+v; want only b", list)
	}
}
