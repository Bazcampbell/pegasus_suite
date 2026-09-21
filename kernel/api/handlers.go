// kernel/api/handlers.go

package api

import (
	"fmt"
	"net/http"
	"pegasus_suite/betting/betmatic"
	"pegasus_suite/logger"
	"pegasus_suite/platform/auth"
	"pegasus_suite/platform/util"
	"strconv"
	"time"
)

const (
	defaultLogLimit = 200
	maxLogLimit     = 2000
	maxSettingsBody = 1 << 20
)

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

func (s *Server) handleGetBookmakers(w http.ResponseWriter, _ *http.Request) {
	util.WriteJSON(w, http.StatusOK, betmatic.BookmakerIcons)
}

// reads in-mem ring
// admins see everything, else pinned to user ID
// GET /api/logs?level=ERROR&app=pegasus&process=p1&since=2026-09-17T07:00:00Z&limit=200
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	qs := r.URL.Query()
	q := logger.Query{
		Level:       qs.Get("level"),
		Application: qs.Get("app"),
		ProcessID:   qs.Get("process"),
		UserID:      qs.Get("user"),
		Limit:       defaultLogLimit,
	}

	if !claims.IsAdmin() {
		q.UserID = claims.UserID()
	}

	if v := qs.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			http.Error(w, "since must be RFC3339", http.StatusBadRequest)
			return
		}
		q.Since = t
	}

	if v := qs.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
			return
		}
		q.Limit = min(n, maxLogLimit)
	}

	util.WriteJSON(w, http.StatusOK, logger.Recent(q))
}

func (s *Server) handleSystemStatus(w http.ResponseWriter, _ *http.Request) {
	util.WriteJSON(w, http.StatusOK, s.kernel.Status())
}

// PROCESS HANDLERS
//
//	GET        /api/{app}/processes[?userId=]                 [{id, status}] for the user
//	GET|PUT    /api/{app}/settings?processId=[&userId=]       process settings
//	DELETE     /api/{app}/settings?processId=[&userId=]       delete process, state, settings
//	GET|PUT    /api/system/settings/{name}                    admin: triples, tpd, betfair, betmatic, …
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
		logger.Warn(logger.Log{FormattedMessage: fmt.Sprintf("%s settings not saved error=%v", name, err)})
		http.Error(w, err.Error(), errStatus(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
