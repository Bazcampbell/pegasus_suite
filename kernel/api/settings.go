// kernel/api/settings.go
//
// Settings documents for ADMIN. A PUT is the whole document; the kernel
// validates it against the owning type before it is written. userId is
// admin only; everyone else acts on their own processes.
//
//	GET        /api/{app}/processes[?userId=]                 [{id, status}] for the user
//	GET|PUT    /api/{app}/settings?processId=[&userId=]       one process's document
//	DELETE     /api/{app}/settings?processId=[&userId=]       the process, its state and its document
//	GET|PUT    /api/system/settings/{name}                    admin: triples, tpd, betfair, betmatic, …

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"pegasus_suite/logger"
	"pegasus_suite/platform/util"
)

// Betfair certs are the largest thing in a document; this leaves plenty.
const maxSettingsBody = 1 << 20

func (s *Server) handleListProcesses(w http.ResponseWriter, r *http.Request) {
	app, userID, ok := s.appUser(w, r)
	if !ok {
		return
	}

	list, err := s.kernel.ListProcesses(app, userID)
	if err != nil {
		http.Error(w, err.Error(), errStatus(err))
		return
	}
	util.WriteJSON(w, http.StatusOK, list)
}

func (s *Server) handleDeleteProcessSettings(w http.ResponseWriter, r *http.Request) {
	key, ok := s.processKey(w, r)
	if !ok {
		return
	}

	if err := s.kernel.DeleteProcessSettings(key); err != nil {
		logger.Error(logger.ErrorLog{
			Message:   fmt.Sprintf("unable to delete process settings error=%v", err),
			UserID:    key.UserID,
			ProcessID: key.ProcessID,
		})
		http.Error(w, err.Error(), errStatus(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetProcessSettings(w http.ResponseWriter, r *http.Request) {
	key, ok := s.processKey(w, r)
	if !ok {
		return
	}

	doc, err := s.kernel.ProcessSettings(key)
	if err != nil {
		http.Error(w, err.Error(), errStatus(err))
		return
	}
	writeDoc(w, doc)
}

func (s *Server) handlePutProcessSettings(w http.ResponseWriter, r *http.Request) {
	key, ok := s.processKey(w, r)
	if !ok {
		return
	}
	doc, ok := readDoc(w, r)
	if !ok {
		return
	}

	if err := s.kernel.SaveProcessSettings(key, doc); err != nil {
		logger.Warn(logger.ErrorLog{
			Message:   fmt.Sprintf("process settings not saved error=%v", err),
			UserID:    key.UserID,
			ProcessID: key.ProcessID,
		})
		http.Error(w, err.Error(), errStatus(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetAppSettings(w http.ResponseWriter, r *http.Request) {
	doc, err := s.kernel.AppSettings(r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), errStatus(err))
		return
	}
	writeDoc(w, doc)
}

func (s *Server) handlePutAppSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	doc, ok := readDoc(w, r)
	if !ok {
		return
	}

	if err := s.kernel.SaveAppSettings(name, doc); err != nil {
		logger.Warn(logger.ErrorLog{Message: fmt.Sprintf("%s settings not saved error=%v", name, err)})
		http.Error(w, err.Error(), errStatus(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
