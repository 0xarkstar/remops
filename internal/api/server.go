package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/security"
	"github.com/0xarkstar/remops/internal/transport"
)

// Server is the HTTP API server.
type Server struct {
	config       *config.Config
	transport    transport.Transport
	profileLevel config.PermissionLevel
	approver     security.Approver
	rateLimiter  *security.RateLimiter
	auditLogger  *security.AuditLogger
	version      string
}

// ServerOption is a functional option for Server.
type ServerOption func(*Server)

// WithProfile sets the permission level for the server.
func WithProfile(level config.PermissionLevel) ServerOption {
	return func(s *Server) { s.profileLevel = level }
}

// WithApprover sets the approver for write operations.
func WithApprover(a security.Approver) ServerOption {
	return func(s *Server) { s.approver = a }
}

// WithRateLimiter sets the rate limiter for write operations.
func WithRateLimiter(rl *security.RateLimiter) ServerOption {
	return func(s *Server) { s.rateLimiter = rl }
}

// WithAuditLogger sets the audit logger.
func WithAuditLogger(al *security.AuditLogger) ServerOption {
	return func(s *Server) { s.auditLogger = al }
}

// WithVersion sets the server version string.
func WithVersion(v string) ServerOption {
	return func(s *Server) { s.version = v }
}

// NewServer creates an HTTP API server backed by cfg and t.
func NewServer(cfg *config.Config, t transport.Transport, opts ...ServerOption) *Server {
	s := &Server{
		config:       cfg,
		transport:    t,
		profileLevel: config.LevelAdmin,
		version:      "0.1.0",
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context, addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/status", s.authMiddleware(s.handleStatus))
	mux.HandleFunc("GET /api/v1/services/{name}/logs", s.authMiddleware(s.handleServiceLogs))
	mux.HandleFunc("POST /api/v1/services/{name}/restart", s.authMiddleware(s.handleServiceAction("restart")))
	mux.HandleFunc("POST /api/v1/services/{name}/stop", s.authMiddleware(s.handleServiceAction("stop")))
	mux.HandleFunc("POST /api/v1/services/{name}/start", s.authMiddleware(s.handleServiceAction("start")))
	mux.HandleFunc("GET /api/v1/hosts/{name}/info", s.authMiddleware(s.handleHostInfo))
	mux.HandleFunc("GET /api/v1/hosts/{name}/disk", s.authMiddleware(s.handleHostDisk))
	mux.HandleFunc("POST /api/v1/hosts/{name}/exec", s.authMiddleware(s.handleHostExec))
	mux.HandleFunc("GET /api/v1/doctor", s.authMiddleware(s.handleDoctor))
	mux.HandleFunc("POST /api/v1/db/{service}/query", s.authMiddleware(s.handleDBQuery))
	mux.HandleFunc("GET /api/v1/version", s.authMiddleware(s.handleVersion))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on context cancellation.
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "api: shutdown error: %v\n", err)
		}
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("api server: %w", err)
	}
	return nil
}

// profileFromRequest reads the X-Remops-Profile header, falling back to server default.
func (s *Server) profileFromRequest(r *http.Request) config.PermissionLevel {
	if p := r.Header.Get("X-Remops-Profile"); p != "" {
		return config.ParseLevel(p)
	}
	return s.profileLevel
}

// jsonResponse writes a JSON response with the given status code.
func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data) //nolint:errcheck
}

// jsonError writes a JSON error response.
func jsonError(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, map[string]string{"error": msg})
}
