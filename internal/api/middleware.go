package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// authMiddleware validates the API key from the Authorization header.
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.config.API == nil || s.config.API.APIKey == "" {
			jsonError(w, http.StatusInternalServerError, "api not configured")
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			jsonError(w, http.StatusUnauthorized, "missing Authorization header")
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth || subtle.ConstantTimeCompare([]byte(token), []byte(s.config.API.APIKey)) != 1 {
			jsonError(w, http.StatusUnauthorized, "invalid API key")
			return
		}

		// Limit request body to 1MB to prevent OOM from large payloads.
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		next(w, r)
	}
}
