// kernel/api/processHandlers.go

package api

import (
	"fmt"
	"net/http"
	"pegasus_suite/clients"
	"pegasus_suite/logger"
	"pegasus_suite/platform/util"
)

// PROCESS HANDLERS
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
		logger.Error(logger.Log{
			Application:      key.App,
			FormattedMessage: fmt.Sprintf("unable to delete process settings error=%v", err),
			UserID:           key.UserID,
			ProcessID:        key.ProcessID,
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

	doc, err := s.kernel.GetProcessSettings(key)
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
		logger.Warn(logger.Log{
			Application:      key.App,
			FormattedMessage: fmt.Sprintf("process settings not saved error=%v", err),
			UserID:           key.UserID,
			ProcessID:        key.ProcessID,
		})
		http.Error(w, err.Error(), errStatus(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleProcessStatus(w http.ResponseWriter, r *http.Request) {
	key, ok := s.processKey(w, r)
	if !ok {
		return
	}

	running, err := s.kernel.ProcessRunning(key)
	if err != nil {
		http.Error(w, err.Error(), errStatus(err))
		return
	}

	status := "stopped"
	if running {
		status = "active"
	}
	util.WriteJSON(w, http.StatusOK, map[string]string{"status": status})
}

// appUser plus the processId. Returns 4xx if anything missing.
func (s *Server) processKey(w http.ResponseWriter, r *http.Request) (clients.ProcessKey, bool) {
	app, userID, ok := s.appUser(w, r)
	if !ok {
		return clients.ProcessKey{}, false
	}

	processID := r.URL.Query().Get("processId")
	if msg := checkID(processID); msg != "" {
		http.Error(w, "process ID "+msg, http.StatusBadRequest)
		return clients.ProcessKey{}, false
	}

	return clients.ProcessKey{App: app, UserID: userID, ProcessID: processID}, true
}
