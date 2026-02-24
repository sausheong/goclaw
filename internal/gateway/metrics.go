package gateway

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics collects gateway operational metrics in Prometheus text format.
type Metrics struct {
	requestsTotal   atomic.Int64
	wsConnections   atomic.Int64
	wsMessagesTotal atomic.Int64
	toolCallsTotal  atomic.Int64
	llmCallsTotal   atomic.Int64
	errorsTotal     atomic.Int64
	startTime       time.Time

	toolCounts map[string]*atomic.Int64
	mu         sync.RWMutex
}

// NewMetrics creates a new metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{
		startTime:  time.Now(),
		toolCounts: make(map[string]*atomic.Int64),
	}
}

// IncRequests increments the HTTP request counter.
func (m *Metrics) IncRequests() { m.requestsTotal.Add(1) }

// IncWSConnections increments the active WebSocket connection counter.
func (m *Metrics) IncWSConnections() { m.wsConnections.Add(1) }

// DecWSConnections decrements the active WebSocket connection counter.
func (m *Metrics) DecWSConnections() { m.wsConnections.Add(-1) }

// IncWSMessages increments the WebSocket message counter.
func (m *Metrics) IncWSMessages() { m.wsMessagesTotal.Add(1) }

// IncToolCalls increments the tool call counter for a specific tool.
func (m *Metrics) IncToolCalls(toolName string) {
	m.toolCallsTotal.Add(1)

	m.mu.RLock()
	counter, ok := m.toolCounts[toolName]
	m.mu.RUnlock()

	if !ok {
		m.mu.Lock()
		counter, ok = m.toolCounts[toolName]
		if !ok {
			counter = &atomic.Int64{}
			m.toolCounts[toolName] = counter
		}
		m.mu.Unlock()
	}

	counter.Add(1)
}

// IncLLMCalls increments the LLM call counter.
func (m *Metrics) IncLLMCalls() { m.llmCallsTotal.Add(1) }

// IncErrors increments the error counter.
func (m *Metrics) IncErrors() { m.errorsTotal.Add(1) }

// Handler returns an HTTP handler that serves Prometheus-compatible metrics.
func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		var b strings.Builder

		b.WriteString("# HELP goclaw_uptime_seconds Time since gateway started.\n")
		b.WriteString("# TYPE goclaw_uptime_seconds gauge\n")
		fmt.Fprintf(&b, "goclaw_uptime_seconds %.1f\n\n", time.Since(m.startTime).Seconds())

		b.WriteString("# HELP goclaw_http_requests_total Total HTTP requests.\n")
		b.WriteString("# TYPE goclaw_http_requests_total counter\n")
		fmt.Fprintf(&b, "goclaw_http_requests_total %d\n\n", m.requestsTotal.Load())

		b.WriteString("# HELP goclaw_ws_connections_active Active WebSocket connections.\n")
		b.WriteString("# TYPE goclaw_ws_connections_active gauge\n")
		fmt.Fprintf(&b, "goclaw_ws_connections_active %d\n\n", m.wsConnections.Load())

		b.WriteString("# HELP goclaw_ws_messages_total Total WebSocket messages received.\n")
		b.WriteString("# TYPE goclaw_ws_messages_total counter\n")
		fmt.Fprintf(&b, "goclaw_ws_messages_total %d\n\n", m.wsMessagesTotal.Load())

		b.WriteString("# HELP goclaw_tool_calls_total Total tool calls.\n")
		b.WriteString("# TYPE goclaw_tool_calls_total counter\n")
		fmt.Fprintf(&b, "goclaw_tool_calls_total %d\n\n", m.toolCallsTotal.Load())

		b.WriteString("# HELP goclaw_llm_calls_total Total LLM API calls.\n")
		b.WriteString("# TYPE goclaw_llm_calls_total counter\n")
		fmt.Fprintf(&b, "goclaw_llm_calls_total %d\n\n", m.llmCallsTotal.Load())

		b.WriteString("# HELP goclaw_errors_total Total errors.\n")
		b.WriteString("# TYPE goclaw_errors_total counter\n")
		fmt.Fprintf(&b, "goclaw_errors_total %d\n\n", m.errorsTotal.Load())

		// Per-tool breakdown
		m.mu.RLock()
		if len(m.toolCounts) > 0 {
			b.WriteString("# HELP goclaw_tool_calls_by_tool Tool calls by tool name.\n")
			b.WriteString("# TYPE goclaw_tool_calls_by_tool counter\n")

			// Sort tool names for deterministic output
			names := make([]string, 0, len(m.toolCounts))
			for name := range m.toolCounts {
				names = append(names, name)
			}
			sort.Strings(names)

			for _, name := range names {
				fmt.Fprintf(&b, "goclaw_tool_calls_by_tool{tool=%q} %d\n", name, m.toolCounts[name].Load())
			}
		}
		m.mu.RUnlock()

		w.Write([]byte(b.String()))
	}
}
