// clients/doc/doc.go
//
// The client store as JSON documents in a blob bucket. Layout:
//
//	settings/apps/<name>.json                         admin-level, saved from ADMIN
//	settings/processes/<app>/<user_id>/<pid>.json     one process, saved from ADMIN
//	state/<app>/<user_id>/<pid>.json                  {"state": "running"}, set by process ops
//
// The kernel is the only writer. Settings are saved through the settings
// routes, validated first; state is written as processes start and stop.
// They are separate objects so a settings save never races a state write.

package doc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"pegasus_suite/clients"
	"pegasus_suite/platform/blob"
)

const opTimeout = 10 * time.Second

type Store struct {
	bucket blob.Bucket
}

func New(bucket blob.Bucket) *Store { return &Store{bucket: bucket} }

func AppSettingsKey(name string) string { return "settings/apps/" + name + ".json" }

func ProcessKey(k clients.ProcessKey) string {
	return fmt.Sprintf("settings/processes/%s/%s/%s.json", k.App, k.UserID, k.ProcessID)
}

func StateKey(k clients.ProcessKey) string {
	return fmt.Sprintf("state/%s/%s/%s.json", k.App, k.UserID, k.ProcessID)
}

func (s *Store) Process(key clients.ProcessKey) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	data, err := s.bucket.Get(ctx, ProcessKey(key))
	if errors.Is(err, blob.ErrNotFound) {
		return nil, clients.ErrNotFound
	}
	return data, err
}

func (s *Store) AppSettings(name string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	data, err := s.bucket.Get(ctx, AppSettingsKey(name))
	if errors.Is(err, blob.ErrNotFound) {
		return json.RawMessage("{}"), nil
	}
	return data, err
}

func (s *Store) PutProcess(key clients.ProcessKey, doc json.RawMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	return s.bucket.Put(ctx, ProcessKey(key), doc)
}

func (s *Store) PutAppSettings(name string, doc json.RawMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	return s.bucket.Put(ctx, AppSettingsKey(name), doc)
}

func (s *Store) ProcessIDs(application, userID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	prefix := fmt.Sprintf("settings/processes/%s/%s/", application, userID)
	keys, err := s.bucket.List(ctx, prefix)
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, key := range keys {
		// settings/processes/<app>/<user>/<pid>.json
		pid := strings.TrimPrefix(key, prefix)
		if !strings.HasSuffix(pid, ".json") || strings.Contains(pid, "/") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(pid, ".json"))
	}
	return ids, nil
}

func (s *Store) DeleteProcess(key clients.ProcessKey) error {
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	return s.bucket.Delete(ctx, ProcessKey(key))
}

type stateDoc struct {
	State clients.State `json:"state"`
}

func (s *Store) Processes(application string) ([]clients.ProcessRef, error) {
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	prefix := "state/" + application + "/"
	keys, err := s.bucket.List(ctx, prefix)
	if err != nil {
		return nil, err
	}

	var out []clients.ProcessRef
	for _, key := range keys {
		// state/<app>/<user>/<pid>.json
		rest := strings.TrimSuffix(strings.TrimPrefix(key, prefix), ".json")
		user, pid, ok := strings.Cut(rest, "/")
		if !ok || strings.Contains(pid, "/") {
			continue
		}

		data, err := s.bucket.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		var d stateDoc
		if err := json.Unmarshal(data, &d); err != nil {
			continue
		}

		out = append(out, clients.ProcessRef{
			Key:   clients.ProcessKey{App: application, UserID: user, ProcessID: pid},
			State: d.State,
		})
	}
	return out, nil
}

func (s *Store) SetState(key clients.ProcessKey, state clients.State) error {
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	body, _ := json.Marshal(stateDoc{State: state})
	return s.bucket.Put(ctx, StateKey(key), body)
}

func (s *Store) Forget(key clients.ProcessKey) error {
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	return s.bucket.Delete(ctx, StateKey(key))
}

var _ clients.Store = (*Store)(nil)
