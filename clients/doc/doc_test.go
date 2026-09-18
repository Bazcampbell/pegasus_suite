package doc_test

import (
	"encoding/json"
	"errors"
	"testing"

	"pegasus_suite/clients"
	"pegasus_suite/clients/doc"
	"pegasus_suite/platform/blob"
)

// The document store over a directory is what dev runs on and what S3 sees;
// this pins the layout and the not-found behaviour.
func TestDocStoreRoundTrip(t *testing.T) {
	bucket, err := blob.OpenFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := doc.New(bucket)

	key := clients.ProcessKey{App: "pegasus", UserID: "u1", ProcessID: "p1"}

	if _, err := s.Process(key); !errors.Is(err, clients.ErrNotFound) {
		t.Fatalf("missing process: err = %v, want ErrNotFound", err)
	}
	if got, err := s.AppSettings("pegasus"); err != nil || string(got) != "{}" {
		t.Fatalf("missing app doc = %s, %v; want {} and nil", got, err)
	}

	body := json.RawMessage(`{"betmatic":{"username":"a@b.c"}}`)
	if err := bucket.Put(t.Context(), doc.ProcessKey(key), body); err != nil {
		t.Fatal(err)
	}
	got, err := s.Process(key)
	if err != nil || string(got) != string(body) {
		t.Fatalf("process doc = %s, %v", got, err)
	}

	// Nothing is listed until the kernel records a state.
	if refs, _ := s.Processes("pegasus"); len(refs) != 0 {
		t.Fatalf("listed %d processes before any state was set", len(refs))
	}

	if err := s.SetState(key, clients.StateRunning); err != nil {
		t.Fatal(err)
	}
	refs, err := s.Processes("pegasus")
	if err != nil || len(refs) != 1 || refs[0].Key != key || refs[0].State != clients.StateRunning {
		t.Fatalf("processes = %+v, %v", refs, err)
	}

	if refs, _ := s.Processes("davo"); len(refs) != 0 {
		t.Errorf("another app's state leaked: %+v", refs)
	}

	if err := s.Forget(key); err != nil {
		t.Fatal(err)
	}
	if refs, _ := s.Processes("pegasus"); len(refs) != 0 {
		t.Errorf("forgotten process still listed: %+v", refs)
	}

	// Forgetting state never touches the settings document.
	if _, err := s.Process(key); err != nil {
		t.Errorf("settings document lost on Forget: %v", err)
	}
}
