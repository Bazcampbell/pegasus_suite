// kernel/api/util.go

package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"pegasus_suite/kernel"
	"strings"
)

func checkID(id string) string {
	switch {
	case id == "":
		return "missing"
	case len(id) > 128:
		return "too long"
	case strings.ContainsAny(id, `/\`) || strings.Contains(id, ".."):
		return "invalid"
	}
	return ""
}

func readDoc(w http.ResponseWriter, r *http.Request) (json.RawMessage, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSettingsBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, "settings document too large", http.StatusRequestEntityTooLarge)
			return nil, false
		}
		http.Error(w, "unable to read body", http.StatusBadRequest)
		return nil, false
	}
	return body, true
}

func writeDoc(w http.ResponseWriter, doc json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(doc)
}

// kernel err:http err
func errStatus(err error) int {
	switch {
	case errors.Is(err, kernel.ErrAlreadyRunning), errors.Is(err, kernel.ErrNotRunning),
		errors.Is(err, kernel.ErrAppDown), errors.Is(err, kernel.ErrExists):
		return http.StatusConflict
	case errors.Is(err, kernel.ErrNotFound), errors.Is(err, kernel.ErrUnknownApp), errors.Is(err, kernel.ErrNoSettings),
		errors.Is(err, kernel.ErrUnknownSettings):
		return http.StatusNotFound
	case errors.Is(err, kernel.ErrInvalidSettings):
		return http.StatusBadRequest
	case errors.Is(err, kernel.ErrNotReloaded):
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}
