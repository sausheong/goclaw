package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/sausheong/goclaw/internal/agent"
	"github.com/sausheong/goclaw/internal/config"
	"github.com/sausheong/goclaw/internal/llm"
	"github.com/sausheong/goclaw/internal/session"
	"github.com/sausheong/goclaw/internal/tools"
)

// JSONRPCRequest is a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      any             `json:"id"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
	ID      any    `json:"id"`
}

// WebSocketHandler handles WebSocket connections and JSON-RPC dispatch.
type WebSocketHandler struct {
	providers    map[string]llm.LLMProvider
	tools        *tools.Registry
	sessionStore *session.Store
	config       *config.Config
	upgrader     websocket.Upgrader
	mu           sync.RWMutex
}

// NewWebSocketHandler creates a new WebSocket handler.
func NewWebSocketHandler(
	providers map[string]llm.LLMProvider,
	toolReg *tools.Registry,
	sessionStore *session.Store,
	cfg *config.Config,
) *WebSocketHandler {
	return &WebSocketHandler{
		providers:    providers,
		tools:        toolReg,
		sessionStore: sessionStore,
		config:       cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // default; overridden by SetOriginChecker
			},
		},
	}
}

// SetOriginChecker sets the WebSocket origin validation function.
func (h *WebSocketHandler) SetOriginChecker(check func(*http.Request) bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.upgrader.CheckOrigin = check
}

// UpdateConfig hot-reloads the config.
func (h *WebSocketHandler) UpdateConfig(cfg *config.Config) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.config = cfg
}

// Handle upgrades an HTTP connection to WebSocket and processes messages.
func (h *WebSocketHandler) Handle(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	slog.Info("websocket client connected", "remote", r.RemoteAddr)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Error("websocket read error", "error", err)
			}
			return
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			writeJSON(conn, JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   map[string]any{"code": -32700, "message": "Parse error"},
				ID:      nil,
			})
			continue
		}

		h.dispatch(conn, req)
	}
}

func (h *WebSocketHandler) dispatch(conn *websocket.Conn, req JSONRPCRequest) {
	switch req.Method {
	case "chat.send":
		h.handleChatSend(conn, req)
	case "agent.status":
		h.handleAgentStatus(conn, req)
	case "session.list":
		h.handleSessionList(conn, req)
	default:
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32601, "message": "Method not found"},
			ID:      req.ID,
		})
	}
}

type chatSendParams struct {
	AgentID string `json:"agentId"`
	Text    string `json:"text"`
}

func (h *WebSocketHandler) handleChatSend(conn *websocket.Conn, req JSONRPCRequest) {
	var params chatSendParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32602, "message": "Invalid params"},
			ID:      req.ID,
		})
		return
	}

	if params.AgentID == "" {
		params.AgentID = "default"
	}

	h.mu.RLock()
	agentCfg, ok := h.config.GetAgent(params.AgentID)
	h.mu.RUnlock()

	if !ok {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32602, "message": "Unknown agent"},
			ID:      req.ID,
		})
		return
	}

	// Resolve LLM provider
	providerName, modelName := llm.ParseProviderModel(agentCfg.Model)
	provider, ok := h.providers[providerName]
	if !ok {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": "LLM provider not configured: " + providerName},
			ID:      req.ID,
		})
		return
	}

	// Load or create session
	sessionKey := "ws_default"
	sess, err := h.sessionStore.Load(params.AgentID, sessionKey)
	if err != nil {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": "Session error: " + err.Error()},
			ID:      req.ID,
		})
		return
	}

	// Run agent
	rt := &agent.Runtime{
		LLM:       provider,
		Tools:     h.tools,
		Session:   sess,
		Model:     modelName,
		Workspace: agentCfg.Workspace,
	}

	events, err := rt.Run(context.Background(), params.Text, nil)
	if err != nil {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": err.Error()},
			ID:      req.ID,
		})
		return
	}

	for event := range events {
		var result any
		switch event.Type {
		case agent.EventTextDelta:
			result = map[string]any{"type": "text_delta", "text": event.Text}
		case agent.EventToolCallStart:
			result = map[string]any{"type": "tool_call_start", "tool": event.ToolCall.Name, "id": event.ToolCall.ID}
		case agent.EventToolResult:
			result = map[string]any{"type": "tool_result", "tool": event.ToolCall.Name, "output": event.Result.Output, "error": event.Result.Error}
		case agent.EventDone:
			result = map[string]any{"type": "done"}
		case agent.EventError:
			result = map[string]any{"type": "error", "message": event.Error.Error()}
		}
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  result,
			ID:      req.ID,
		})
	}
}

func (h *WebSocketHandler) handleAgentStatus(conn *websocket.Conn, req JSONRPCRequest) {
	h.mu.RLock()
	agents := h.config.Agents.List
	h.mu.RUnlock()

	var statuses []map[string]any
	for _, a := range agents {
		statuses = append(statuses, map[string]any{
			"id":        a.ID,
			"name":      a.Name,
			"model":     a.Model,
			"workspace": a.Workspace,
		})
	}

	writeJSON(conn, JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  map[string]any{"agents": statuses},
		ID:      req.ID,
	})
}

func (h *WebSocketHandler) handleSessionList(conn *websocket.Conn, req JSONRPCRequest) {
	// Basic implementation — list is limited for Phase 1
	writeJSON(conn, JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  map[string]any{"sessions": []any{}},
		ID:      req.ID,
	})
}

func writeJSON(conn *websocket.Conn, v any) {
	if err := conn.WriteJSON(v); err != nil {
		slog.Error("websocket write error", "error", err)
	}
}
