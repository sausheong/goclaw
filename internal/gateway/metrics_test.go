package gateway

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricsHandler(t *testing.T) {
	m := NewMetrics()

	// Increment some counters
	m.IncRequests()
	m.IncRequests()
	m.IncWSConnections()
	m.IncWSMessages()
	m.IncToolCalls("bash")
	m.IncToolCalls("bash")
	m.IncToolCalls("read_file")
	m.IncLLMCalls()
	m.IncErrors()

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	body := w.Body.String()

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, body, "goclaw_http_requests_total 2")
	assert.Contains(t, body, "goclaw_ws_connections_active 1")
	assert.Contains(t, body, "goclaw_ws_messages_total 1")
	assert.Contains(t, body, "goclaw_tool_calls_total 3")
	assert.Contains(t, body, "goclaw_llm_calls_total 1")
	assert.Contains(t, body, "goclaw_errors_total 1")
	assert.Contains(t, body, `goclaw_tool_calls_by_tool{tool="bash"} 2`)
	assert.Contains(t, body, `goclaw_tool_calls_by_tool{tool="read_file"} 1`)
	assert.Contains(t, body, "goclaw_uptime_seconds")

	// Check Content-Type
	assert.True(t, strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain"))
}

func TestMetricsDecWSConnections(t *testing.T) {
	m := NewMetrics()
	m.IncWSConnections()
	m.IncWSConnections()
	m.DecWSConnections()

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	m.Handler()(w, req)

	assert.Contains(t, w.Body.String(), "goclaw_ws_connections_active 1")
}
