package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

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
	jobScheduler tools.JobScheduler
	activeRuns   map[*websocket.Conn]context.CancelFunc
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
		activeRuns:   make(map[*websocket.Conn]context.CancelFunc),
		upgrader: websocket.Upgrader{
			CheckOrigin: AllowedOrigins(nil), // default: localhost-only; overridden by SetOriginChecker
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

// SetJobScheduler sets the job scheduler for jobs.* RPC methods.
func (h *WebSocketHandler) SetJobScheduler(js tools.JobScheduler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.jobScheduler = js
}

// Handle upgrades an HTTP connection to WebSocket and processes messages.
func (h *WebSocketHandler) Handle(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	conn.SetReadLimit(1 * 1024 * 1024) // 1MB max message size

	slog.Info("websocket client connected", "remote", r.RemoteAddr)
	defer func() {
		// Cancel any active run for this connection to prevent orphaned goroutines
		h.mu.Lock()
		if cancel, ok := h.activeRuns[conn]; ok {
			cancel()
			delete(h.activeRuns, conn)
		}
		h.mu.Unlock()
	}()

	// Per-connection rate limiter: max 30 messages per second.
	// Uses a token bucket that refills at 30 tokens/sec with burst of 30.
	const rateLimit = 30
	tokens := rateLimit
	lastRefill := time.Now()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Error("websocket read error", "error", err)
			}
			return
		}

		// Refill tokens based on elapsed time
		now := time.Now()
		elapsed := now.Sub(lastRefill)
		tokens += int(elapsed.Seconds() * rateLimit)
		if tokens > rateLimit {
			tokens = rateLimit
		}
		lastRefill = now

		if tokens <= 0 {
			writeJSON(conn, JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   map[string]any{"code": -32000, "message": "rate limit exceeded"},
				ID:      nil,
			})
			continue
		}
		tokens--

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
	case "chat.abort":
		h.handleChatAbort(conn, req)
	case "agent.status":
		h.handleAgentStatus(conn, req)
	case "session.list":
		h.handleSessionList(conn, req)
	case "session.history":
		h.handleSessionHistory(conn, req)
	case "session.clear":
		h.handleSessionClear(conn, req)
	case "jobs.list":
		h.handleJobsList(conn, req)
	case "jobs.pause":
		h.handleJobsPause(conn, req)
	case "jobs.resume":
		h.handleJobsResume(conn, req)
	case "jobs.remove":
		h.handleJobsRemove(conn, req)
	case "jobs.update":
		h.handleJobsUpdate(conn, req)
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

	// Apply agent tool policy
	var executor tools.Executor = h.tools
	if len(agentCfg.Tools.Allow) > 0 || len(agentCfg.Tools.Deny) > 0 {
		executor = tools.NewFilteredRegistry(h.tools, tools.Policy{
			Allow: agentCfg.Tools.Allow,
			Deny:  agentCfg.Tools.Deny,
		})
	}

	// Run agent
	rt := &agent.Runtime{
		LLM:          provider,
		Tools:        executor,
		Session:      sess,
		AgentID:      agentCfg.ID,
		AgentName:    agentCfg.Name,
		Model:        modelName,
		Workspace:    agentCfg.Workspace,
		MaxTurns:     agentCfg.MaxTurns,
		SystemPrompt: agentCfg.SystemPrompt,
	}

	runCtx, runCancel := context.WithCancel(context.Background())

	// Track this run so chat.abort and disconnect can cancel it
	h.mu.Lock()
	h.activeRuns[conn] = runCancel
	h.mu.Unlock()

	events, err := rt.Run(runCtx, params.Text, nil)
	if err != nil {
		runCancel()
		h.mu.Lock()
		delete(h.activeRuns, conn)
		h.mu.Unlock()
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": err.Error()},
			ID:      req.ID,
		})
		return
	}

	// Stream events in a goroutine so the WebSocket read loop stays free
	// to process chat.abort messages.
	go func() {
		defer func() {
			runCancel()
			h.mu.Lock()
			delete(h.activeRuns, conn)
			h.mu.Unlock()
		}()

		for event := range events {
			var result any
			switch event.Type {
			case agent.EventTextDelta:
				result = map[string]any{"type": "text_delta", "text": event.Text}
			case agent.EventToolCallStart:
				result = map[string]any{"type": "tool_call_start", "tool": event.ToolCall.Name, "id": event.ToolCall.ID, "input": event.ToolCall.Input}
			case agent.EventToolResult:
				r := map[string]any{"type": "tool_result", "tool": event.ToolCall.Name, "id": event.ToolCall.ID, "input": event.ToolCall.Input, "output": event.Result.Output, "error": event.Result.Error}
				if len(event.Result.Images) > 0 {
					var imgs []map[string]string
					for _, img := range event.Result.Images {
						imgs = append(imgs, map[string]string{
							"mimeType": img.MimeType,
							"data":     base64.StdEncoding.EncodeToString(img.Data),
						})
					}
					r["images"] = imgs
				}
				result = r
			case agent.EventDone:
				result = map[string]any{"type": "done"}
			case agent.EventError:
				result = map[string]any{"type": "error", "message": event.Error.Error()}
			case agent.EventAborted:
				result = map[string]any{"type": "aborted"}
			}
			writeJSON(conn, JSONRPCResponse{
				JSONRPC: "2.0",
				Result:  result,
				ID:      req.ID,
			})
		}
	}()
}

func (h *WebSocketHandler) handleChatAbort(conn *websocket.Conn, req JSONRPCRequest) {
	h.mu.RLock()
	cancel, ok := h.activeRuns[conn]
	h.mu.RUnlock()

	if ok {
		cancel()
	}

	writeJSON(conn, JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  map[string]any{"ok": true},
		ID:      req.ID,
	})
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

type sessionParams struct {
	AgentID string `json:"agentId"`
}

func (h *WebSocketHandler) handleSessionHistory(conn *websocket.Conn, req JSONRPCRequest) {
	var params sessionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		params.AgentID = "default"
	}
	if params.AgentID == "" {
		params.AgentID = "default"
	}

	sess, err := h.sessionStore.Load(params.AgentID, "ws_default")
	if err != nil {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": "Session load error: " + err.Error()},
			ID:      req.ID,
		})
		return
	}

	history := sess.History()
	var entries []map[string]any

	for _, entry := range history {
		switch entry.Type {
		case session.EntryTypeMessage:
			var msg session.MessageData
			if err := json.Unmarshal(entry.Data, &msg); err != nil {
				continue
			}
			entries = append(entries, map[string]any{
				"type": "message",
				"role": entry.Role,
				"text": msg.Text,
			})
		case session.EntryTypeToolCall:
			var tc session.ToolCallData
			if err := json.Unmarshal(entry.Data, &tc); err != nil {
				continue
			}
			entries = append(entries, map[string]any{
				"type":  "tool_call",
				"tool":  tc.Tool,
				"id":    tc.ID,
				"input": tc.Input,
			})
		case session.EntryTypeToolResult:
			var tr session.ToolResultData
			if err := json.Unmarshal(entry.Data, &tr); err != nil {
				continue
			}
			e := map[string]any{
				"type":         "tool_result",
				"tool_call_id": tr.ToolCallID,
				"output":       tr.Output,
				"error":        tr.Error,
			}
			if len(tr.Images) > 0 {
				var imgs []map[string]string
				for _, img := range tr.Images {
					imgs = append(imgs, map[string]string{
						"mimeType": img.MimeType,
						"data":     img.Data, // already base64
					})
				}
				e["images"] = imgs
			}
			entries = append(entries, e)
		case session.EntryTypeMeta:
			// Skip compaction summaries — internal
		}
	}

	writeJSON(conn, JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  map[string]any{"entries": entries},
		ID:      req.ID,
	})
}

func (h *WebSocketHandler) handleSessionClear(conn *websocket.Conn, req JSONRPCRequest) {
	var params sessionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		params.AgentID = "default"
	}
	if params.AgentID == "" {
		params.AgentID = "default"
	}

	if err := h.sessionStore.Delete(params.AgentID, "ws_default"); err != nil {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": "Delete error: " + err.Error()},
			ID:      req.ID,
		})
		return
	}

	writeJSON(conn, JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  map[string]any{"ok": true},
		ID:      req.ID,
	})
}

// jobs.* handlers

type jobNameParams struct {
	Name string `json:"name"`
}

type jobUpdateParams struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
}

func (h *WebSocketHandler) handleJobsList(conn *websocket.Conn, req JSONRPCRequest) {
	h.mu.RLock()
	js := h.jobScheduler
	h.mu.RUnlock()

	if js == nil {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": "Job scheduler not available"},
			ID:      req.ID,
		})
		return
	}

	jobs := js.ListJobs()
	writeJSON(conn, JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  map[string]any{"jobs": jobs},
		ID:      req.ID,
	})
}

func (h *WebSocketHandler) handleJobsPause(conn *websocket.Conn, req JSONRPCRequest) {
	h.mu.RLock()
	js := h.jobScheduler
	h.mu.RUnlock()

	if js == nil {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": "Job scheduler not available"},
			ID:      req.ID,
		})
		return
	}

	var params jobNameParams
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32602, "message": "Invalid params: name required"},
			ID:      req.ID,
		})
		return
	}

	if err := js.PauseJob(params.Name); err != nil {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": err.Error()},
			ID:      req.ID,
		})
		return
	}

	writeJSON(conn, JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  map[string]any{"ok": true},
		ID:      req.ID,
	})
}

func (h *WebSocketHandler) handleJobsResume(conn *websocket.Conn, req JSONRPCRequest) {
	h.mu.RLock()
	js := h.jobScheduler
	h.mu.RUnlock()

	if js == nil {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": "Job scheduler not available"},
			ID:      req.ID,
		})
		return
	}

	var params jobNameParams
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32602, "message": "Invalid params: name required"},
			ID:      req.ID,
		})
		return
	}

	if err := js.ResumeJob(params.Name); err != nil {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": err.Error()},
			ID:      req.ID,
		})
		return
	}

	writeJSON(conn, JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  map[string]any{"ok": true},
		ID:      req.ID,
	})
}

func (h *WebSocketHandler) handleJobsRemove(conn *websocket.Conn, req JSONRPCRequest) {
	h.mu.RLock()
	js := h.jobScheduler
	h.mu.RUnlock()

	if js == nil {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": "Job scheduler not available"},
			ID:      req.ID,
		})
		return
	}

	var params jobNameParams
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32602, "message": "Invalid params: name required"},
			ID:      req.ID,
		})
		return
	}

	if err := js.RemoveJob(params.Name); err != nil {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": err.Error()},
			ID:      req.ID,
		})
		return
	}

	writeJSON(conn, JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  map[string]any{"ok": true},
		ID:      req.ID,
	})
}

func (h *WebSocketHandler) handleJobsUpdate(conn *websocket.Conn, req JSONRPCRequest) {
	h.mu.RLock()
	js := h.jobScheduler
	h.mu.RUnlock()

	if js == nil {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": "Job scheduler not available"},
			ID:      req.ID,
		})
		return
	}

	var params jobUpdateParams
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" || params.Schedule == "" {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32602, "message": "Invalid params: name and schedule required"},
			ID:      req.ID,
		})
		return
	}

	if err := js.UpdateJobSchedule(params.Name, params.Schedule); err != nil {
		writeJSON(conn, JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32603, "message": err.Error()},
			ID:      req.ID,
		})
		return
	}

	writeJSON(conn, JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  map[string]any{"ok": true},
		ID:      req.ID,
	})
}

func writeJSON(conn *websocket.Conn, v any) {
	if err := conn.WriteJSON(v); err != nil {
		slog.Error("websocket write error", "error", err)
	}
}
