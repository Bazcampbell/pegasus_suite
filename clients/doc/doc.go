// clients/doc/doc.go
//
// The client store as JSON documents in a blob bucket. Layout:
//
//	settings/apps/<scope>.json                        admin-level, written by ADMIN
//	settings/processes/<app>/<user_id>/<pid>.json     one process, written by ADMIN
//	state/<app>/<user_id>/<pid>.json                  {"state": "running"}, written by the kernel
//
// Settings and state are separate objects on purpose: ADMIN and the kernel
// never write the same document, so neither can clobber the other.

package doc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"racing_wagering/clients"
	"racing_wagering/platform/blob"
)

const opTimeout = 10 * time.Second

type Store struct {
	bucket blob.Bucket
}

func New(bucket blob.Bucket) *Store { return &Store{bucket: bucket} }

func AppKey(scope string) string { return "settings/apps/" + scope + ".json" }

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

func (s *Store) App(scope string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	data, err := s.bucket.Get(ctx, AppKey(scope))
	if errors.Is(err, blob.ErrNotFound) {
		return json.RawMessage("{}"), nil
	}
	return data, err
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
