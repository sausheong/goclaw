package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// ServerOptions configures the gateway server.
type ServerOptions struct {
	AuthToken      string   // bearer token for API auth (empty = no auth)
	AllowedOrigins []string // WebSocket allowed origins (empty = localhost only)
	MetricsHandler http.HandlerFunc // optional /metrics handler
	UIHandler      http.Handler     // optional /ui handler
	ChatHandler    http.HandlerFunc // optional /chat handler
}

// Server is the GoClaw gateway HTTP + WebSocket server.
type Server struct {
	httpServer *http.Server
	router     chi.Router
	wsHandler  *WebSocketHandler
	host       string
	port       int
	opts       ServerOptions
}

// NewServer creates a new gateway server.
func NewServer(host string, port int, wsHandler *WebSocketHandler, opts ...ServerOptions) *Server {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	var o ServerOptions
	if len(opts) > 0 {
		o = opts[0]
	}

	// Add bearer auth if configured
	if o.AuthToken != "" {
		r.Use(BearerAuthMiddleware(o.AuthToken))
	}

	// Set WebSocket origin validator
	wsHandler.SetOriginChecker(AllowedOrigins(o.AllowedOrigins))

	s := &Server{
		router:    r,
		wsHandler: wsHandler,
		host:      host,
		port:      port,
		opts:      o,
	}

	s.routes()
	return s
}

func (s *Server) routes() {
	s.router.Get("/health", s.handleHealth)
	s.router.Get("/ws", s.wsHandler.Handle)

	if s.opts.MetricsHandler != nil {
		s.router.Get("/metrics", s.opts.MetricsHandler)
	}

	if s.opts.UIHandler != nil {
		s.router.Mount("/ui", s.opts.UIHandler)
	}

	if s.opts.ChatHandler != nil {
		s.router.Get("/chat", s.opts.ChatHandler)
		s.router.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/chat", http.StatusFound)
		})
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}

// Start begins listening and serving.
func (s *Server) Start() error {
	addr := net.JoinHostPort(s.host, fmt.Sprintf("%d", s.port))

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // WebSocket needs no write timeout
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("gateway listening", "addr", addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
