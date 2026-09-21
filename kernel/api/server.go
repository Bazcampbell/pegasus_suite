// kernel/api/server.go

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"pegasus_suite/clients"
	"pegasus_suite/kernel"
	"pegasus_suite/logger"
	"pegasus_suite/platform/auth"
	"pegasus_suite/platform/util"
)

type Server struct {
	port   string
	mux    *http.ServeMux
	jwks   *auth.Verifier
	kernel *kernel.Kernel

	httpServer *http.Server
}

func NewServer(port string, authCfg auth.Config, k *kernel.Kernel) (*Server, error) {
	verifier, err := auth.NewVerifier(authCfg)
	if err != nil {
		return nil, err
	}

	s := &Server{port: port, mux: http.NewServeMux(), jwks: verifier, kernel: k}
	s.routes()
	s.httpServer = &http.Server{
		Addr:              ":" + port,
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return s, nil
}

func (s *Server) Run() error {
	slog.Info("api server starting", "port", s.port)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("shutting down api server")
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) routes() {
	// any authenticated user
	// only on their own process
	authed := []struct {
		pattern string
		handler http.HandlerFunc
	}{
		{"POST /api/{app}/add", s.processOp("added", s.kernel.AddProcess)},
		{"POST /api/{app}/start", s.processOp("started", s.kernel.StartProcess)},
		{"POST /api/{app}/stop", s.processOp("stopped", s.kernel.StopProcess)},
		{"POST /api/{app}/restart", s.processOp("restarted", s.kernel.RestartProcess)},
		{"POST /api/{app}/delete", s.processOp("deleted", s.kernel.DeleteProcess)},

		{"GET /api/{app}/status", s.handleProcessStatus},

		{"GET /api/{app}/processes", s.handleListProcesses},

		{"GET /api/{app}/settings", s.handleGetProcessSettings},
		{"PUT /api/{app}/settings", s.handlePutProcessSettings},
		{"DELETE /api/{app}/settings", s.handleDeleteProcessSettings},

		{"GET /api/betting/bookmakers", s.handleGetBookmakers},

		{"GET /api/logs", s.handleLogs},
	}
	for _, r := range authed {
		s.mux.Handle(r.pattern, auth.RequireAuth(s.jwks, r.handler))
	}

	// actual runtime
	adminOnly := []struct {
		pattern string
		handler http.HandlerFunc
	}{
		{"POST /api/system/start", s.systemOp("started", s.kernel.Start)},
		{"POST /api/system/stop", s.systemOp("stopped", s.kernel.Stop)},
		{"POST /api/system/restart", s.systemOp("restarted", s.kernel.Restart)},
		{"GET /api/system/status", s.handleSystemStatus},

		{"GET /api/system/settings/{name}", s.handleGetAppSettings},
		{"PUT /api/system/settings/{name}", s.handlePutAppSettings},
	}
	for _, r := range adminOnly {
		s.mux.Handle(r.pattern, auth.RequireRole(s.jwks, "admin", r.handler))
	}

	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		util.WriteJSON(w, http.StatusOK, map[string]string{"status": "online"})
	})
}

// returns application from path, user from claims
func (s *Server) appUser(w http.ResponseWriter, r *http.Request) (app, userID string, ok bool) {
	app = r.PathValue("app")
	if !s.kernel.HasApp(app) {
		http.Error(w, "unknown application", http.StatusNotFound)
		return "", "", false
	}

	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", "", false
	}

	// only admin can update settings/processes for another user
	userID = claims.UserID()
	if asked := r.URL.Query().Get("userId"); asked != "" && asked != userID {
		if !claims.IsAdmin() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return "", "", false
		}
		if msg := checkID(asked); msg != "" {
			http.Error(w, "user ID "+msg, http.StatusBadRequest)
			return "", "", false
		}
		userID = asked
	}
	return app, userID, true
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

// generic function for a start/stop/restart/delete/add process
func (s *Server) processOp(done string, op func(clients.ProcessKey) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, ok := s.processKey(w, r)
		if !ok {
			return
		}

		if err := op(key); err != nil {
			logger.Error(logger.Log{
				Application:      key.App,
				FormattedMessage: fmt.Sprintf("unable to %s process error=%v", r.PathValue("app")+" "+done, err),
				UserID:           key.UserID,
				ProcessID:        key.ProcessID,
			})
			http.Error(w, err.Error(), errStatus(err))
			return
		}
		util.WriteJSON(w, http.StatusOK, done)
	}
}

func (s *Server) systemOp(done string, op func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if err := op(); err != nil {
			logger.Error(logger.Log{FormattedMessage: fmt.Sprintf("unable to %s runtime error=%v", done, err)})
			http.Error(w, err.Error(), errStatus(err))
			return
		}
		util.WriteJSON(w, http.StatusOK, done)
	}
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
		// saved, but the process did not come back: the caller must see it
		return http.StatusConflict
	}
	return http.StatusInternalServerError
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
