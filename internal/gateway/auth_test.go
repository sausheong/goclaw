package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBearerAuthMiddlewareNoToken(t *testing.T) {
	// When no token is configured, everything is allowed
	handler := BearerAuthMiddleware("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBearerAuthMiddlewareValidToken(t *testing.T) {
	handler := BearerAuthMiddleware("secret123")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBearerAuthMiddlewareInvalidToken(t *testing.T) {
	handler := BearerAuthMiddleware("secret123")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBearerAuthMiddlewareMissingHeader(t *testing.T) {
	handler := BearerAuthMiddleware("secret123")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBearerAuthMiddlewareHealthBypass(t *testing.T) {
	handler := BearerAuthMiddleware("secret123")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBearerAuthMiddlewareQueryParam(t *testing.T) {
	handler := BearerAuthMiddleware("secret123")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/ws?token=secret123", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAllowedOriginsDefault(t *testing.T) {
	check := AllowedOrigins(nil) // default = localhost only

	// Localhost origins should pass
	r := httptest.NewRequest("GET", "/ws", nil)
	r.Header.Set("Origin", "http://127.0.0.1:18789")
	assert.True(t, check(r))

	r = httptest.NewRequest("GET", "/ws", nil)
	r.Header.Set("Origin", "http://localhost:3000")
	assert.True(t, check(r))

	// External origins should fail
	r = httptest.NewRequest("GET", "/ws", nil)
	r.Header.Set("Origin", "http://evil.com")
	assert.False(t, check(r))

	// No origin header should pass (CLI clients)
	r = httptest.NewRequest("GET", "/ws", nil)
	assert.True(t, check(r))
}

func TestAllowedOriginsCustom(t *testing.T) {
	check := AllowedOrigins([]string{"https://mydomain.com", "http://localhost:3000"})

	r := httptest.NewRequest("GET", "/ws", nil)
	r.Header.Set("Origin", "https://mydomain.com")
	assert.True(t, check(r))

	r = httptest.NewRequest("GET", "/ws", nil)
	r.Header.Set("Origin", "https://evil.com")
	assert.False(t, check(r))
}
