// kernel/api/server.go
//
// The control plane. Process routes are generic over the application in the
// path; the kernel resolves the rest.

package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"racing_wagering/betting/betmatic"
	"racing_wagering/clients"
	"racing_wagering/kernel"
	"racing_wagering/logger"
	"racing_wagering/platform/auth"
	"racing_wagering/platform/util"
)

type Server struct {
	port   string
	mux    *http.ServeMux
	jwks   *auth.Verifier
	kernel *kernel.Kernel

	httpServer *http.Server
}

func NewServer(port, jwkURL string, k *kernel.Kernel) (*Server, error) {
	verifier, err := auth.NewVerifier(auth.Config{
		URL:      jwkURL,
		Issuer:   os.Getenv("JWT_ISSUER"),
		Audience: os.Getenv("JWT_AUDIENCE"),
	})
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
	// Any authenticated user, on their own processes of one application.
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
		{"GET /api/betting/bookmakers", s.handleGetBookmakers},
		{"GET /api/logs", s.handleLogs},
	}
	for _, r := range authed {
		s.mux.Handle(r.pattern, auth.RequireAuth(s.jwks, r.handler))
	}

	// Admin only — the whole runtime.
	adminOnly := []struct {
		pattern string
		handler http.HandlerFunc
	}{
		{"POST /api/system/start", s.systemOp("started", s.kernel.Start)},
		{"POST /api/system/stop", s.systemOp("stopped", s.kernel.Stop)},
		{"POST /api/system/restart", s.systemOp("restarted", s.kernel.Restart)},
		{"GET /api/system/status", s.handleSystemStatus},
	}
	for _, r := range adminOnly {
		s.mux.Handle(r.pattern, auth.RequireRole(s.jwks, "admin", r.handler))
	}

	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		util.WriteJSON(w, http.StatusOK, map[string]string{"status": "online"})
	})
}

// processKey pulls the application, the caller, and the process off the
// request, writing the 4xx if anything is missing.
func (s *Server) processKey(w http.ResponseWriter, r *http.Request) (clients.ProcessKey, bool) {
	app := r.PathValue("app")
	if !s.kernel.HasApp(app) {
		http.Error(w, "unknown application", http.StatusNotFound)
		return clients.ProcessKey{}, false
	}

	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return clients.ProcessKey{}, false
	}

	processID := r.URL.Query().Get("processId")
	if processID == "" {
		http.Error(w, "process ID missing", http.StatusBadRequest)
		return clients.ProcessKey{}, false
	}
	if len(processID) > 128 {
		http.Error(w, "process ID too long", http.StatusBadRequest)
		return clients.ProcessKey{}, false
	}

	return clients.ProcessKey{App: app, UserID: claims.UserID(), ProcessID: processID}, true
}

func (s *Server) processOp(done string, op func(clients.ProcessKey) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, ok := s.processKey(w, r)
		if !ok {
			return
		}

		if err := op(key); err != nil {
			logger.Error(logger.ErrorLog{
				Message:   fmt.Sprintf("unable to %s process error=%v", r.PathValue("app")+" "+done, err),
				UserID:    key.UserID,
				ProcessID: key.ProcessID,
			})
			http.Error(w, err.Error(), errStatus(err))
			return
		}
		util.WriteJSON(w, http.StatusOK, done)
	}
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

func (s *Server) handleGetBookmakers(w http.ResponseWriter, _ *http.Request) {
	util.WriteJSON(w, http.StatusOK, betmatic.BookmakerIcons)
}

const (
	defaultLogLimit = 200
	maxLogLimit     = 2000
)

// handleLogs reads the in-memory ring. Admins see everything; anyone else is
// pinned to their own user id whatever they ask for.
//
//	GET /api/logs?level=ERROR&app=pegasus&process=p1&since=2026-09-17T07:00:00Z&limit=200
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

func (s *Server) systemOp(done string, op func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if err := op(); err != nil {
			logger.Error(logger.ErrorLog{Message: fmt.Sprintf("unable to %s runtime error=%v", done, err)})
			http.Error(w, err.Error(), errStatus(err))
			return
		}
		util.WriteJSON(w, http.StatusOK, done)
	}
}

func (s *Server) handleSystemStatus(w http.ResponseWriter, _ *http.Request) {
	util.WriteJSON(w, http.StatusOK, s.kernel.Status())
}

// errStatus maps kernel errors to HTTP. State conflicts are 409 and a missing
// process 404; anything else is the server's fault.
func errStatus(err error) int {
	switch {
	case errors.Is(err, kernel.ErrAlreadyRunning), errors.Is(err, kernel.ErrNotRunning),
		errors.Is(err, kernel.ErrAppDown), errors.Is(err, kernel.ErrExists):
		return http.StatusConflict
	case errors.Is(err, kernel.ErrNotFound), errors.Is(err, kernel.ErrUnknownApp), errors.Is(err, kernel.ErrNoSettings):
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
