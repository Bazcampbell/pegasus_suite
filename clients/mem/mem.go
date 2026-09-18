// clients/mem/mem.go
//
// In-memory client store. Tests and dry runs; nothing survives a restart.

package mem

import (
	"encoding/json"
	"sync"

	"pegasus_suite/clients"
)

type Store struct {
	mu        sync.RWMutex
	processes map[clients.ProcessKey]json.RawMessage
	apps      map[string]json.RawMessage
	states    map[clients.ProcessKey]clients.State
}

func New() *Store {
	return &Store{
		processes: make(map[clients.ProcessKey]json.RawMessage),
		apps:      make(map[string]json.RawMessage),
		states:    make(map[clients.ProcessKey]clients.State),
	}
}

func (s *Store) PutProcess(key clients.ProcessKey, doc json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processes[key] = doc
	return nil
}

func (s *Store) PutAppSettings(name string, doc json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apps[name] = doc
	return nil
}

func (s *Store) Process(key clients.ProcessKey) (json.RawMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.processes[key]
	if !ok {
		return nil, clients.ErrNotFound
	}
	return doc, nil
}

func (s *Store) AppSettings(name string) (json.RawMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if doc, ok := s.apps[name]; ok {
		return doc, nil
	}
	return json.RawMessage("{}"), nil
}

func (s *Store) Processes(application string) ([]clients.ProcessRef, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []clients.ProcessRef
	for key, state := range s.states {
		if key.App == application {
			out = append(out, clients.ProcessRef{Key: key, State: state})
		}
	}
	return out, nil
}

func (s *Store) SetState(key clients.ProcessKey, state clients.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[key] = state
	return nil
}

func (s *Store) Forget(key clients.ProcessKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, key)
	return nil
}

func (s *Store) ProcessIDs(application, userID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var ids []string
	for key := range s.processes {
		if key.App == application && key.UserID == userID {
			ids = append(ids, key.ProcessID)
		}
	}
	return ids, nil
}

func (s *Store) DeleteProcess(key clients.ProcessKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.processes, key)
	return nil
}

var _ clients.Store = (*Store)(nil)
