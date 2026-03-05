package gateway

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// BearerAuthMiddleware returns middleware that validates a Bearer token.
// If token is empty, the middleware is a no-op (no auth required).
func BearerAuthMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if token == "" {
			return next // no auth configured
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Allow health check without auth
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			// Check Authorization header
			auth := r.Header.Get("Authorization")
			if auth == "" {
				// Also check query parameter for WebSocket clients that
				// can't set custom headers
				auth = "Bearer " + r.URL.Query().Get("token")
			}

			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			providedToken := strings.TrimPrefix(auth, "Bearer ")
			if subtle.ConstantTimeCompare([]byte(providedToken), []byte(token)) != 1 {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// AllowedOrigins returns a WebSocket CheckOrigin function that validates
// the request origin against a list of allowed origins.
// If no origins are configured, it defaults to allowing localhost only.
func AllowedOrigins(origins []string) func(r *http.Request) bool {
	if len(origins) == 0 {
		// Default: allow localhost origins only
		return func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // no origin header (e.g. CLI tools, curl)
			}
			return strings.HasPrefix(origin, "http://127.0.0.1") ||
				strings.HasPrefix(origin, "http://localhost") ||
				strings.HasPrefix(origin, "https://127.0.0.1") ||
				strings.HasPrefix(origin, "https://localhost")
		}
	}

	allowed := make(map[string]bool)
	for _, o := range origins {
		allowed[strings.TrimRight(o, "/")] = true
	}

	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		return allowed[strings.TrimRight(origin, "/")]
	}
}
