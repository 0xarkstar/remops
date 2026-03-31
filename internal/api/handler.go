package api

import "net/http"

// Handler returns an http.Handler with all routes registered.
// This is useful for embedding the API in tests or custom HTTP servers
// without calling Run (which binds a real port).
func (s *Server) Handler() http.Handler {
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
	mux.HandleFunc("GET /api/v1/stacks/{name}/ps", s.authMiddleware(s.handleStackPS))
	mux.HandleFunc("GET /api/v1/stacks/{name}/logs", s.authMiddleware(s.handleStackLogs))
	mux.HandleFunc("POST /api/v1/stacks/{name}/up", s.authMiddleware(s.handleStackAction("up -d")))
	mux.HandleFunc("POST /api/v1/stacks/{name}/pull", s.authMiddleware(s.handleStackAction("pull")))
	mux.HandleFunc("POST /api/v1/stacks/{name}/restart", s.authMiddleware(s.handleStackAction("restart")))
	mux.HandleFunc("POST /api/v1/stacks/{name}/down", s.authMiddleware(s.handleStackDown))
	return mux
}
