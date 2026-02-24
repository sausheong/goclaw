package gateway

import (
	"fmt"
	"net/http"
	"time"

	"github.com/sausheong/goclaw/internal/config"
)

// NewUIHandler returns an HTTP handler that serves a minimal control dashboard.
func NewUIHandler(cfg *config.Config, version string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, uiHTML, version, time.Now().UTC().Format(time.RFC3339), renderAgents(cfg))
	})
	return http.StripPrefix("/ui", mux)
}

func renderAgents(cfg *config.Config) string {
	html := ""
	for _, a := range cfg.Agents.List {
		html += fmt.Sprintf(`<tr>
			<td>%s</td>
			<td>%s</td>
			<td>%s</td>
			<td>%s</td>
			<td>%s</td>
		</tr>`, a.ID, a.Name, a.Model, a.Workspace, a.Sandbox)
	}
	return html
}

const uiHTML = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>GoClaw Control Panel</title>
<style>
	* { margin: 0; padding: 0; box-sizing: border-box; }
	body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, monospace; background: #1a1a2e; color: #e0e0e0; padding: 2rem; }
	h1 { color: #16dbaa; margin-bottom: 0.5rem; }
	.subtitle { color: #888; margin-bottom: 2rem; font-size: 0.9rem; }
	.card { background: #16213e; border-radius: 8px; padding: 1.5rem; margin-bottom: 1.5rem; border: 1px solid #0f3460; }
	.card h2 { color: #53a8b6; margin-bottom: 1rem; font-size: 1.1rem; }
	table { width: 100%%; border-collapse: collapse; }
	th, td { text-align: left; padding: 0.5rem 1rem; border-bottom: 1px solid #0f3460; }
	th { color: #53a8b6; font-weight: 600; }
	td { color: #ccc; }
	.status { display: inline-block; width: 8px; height: 8px; border-radius: 50%%; background: #16dbaa; margin-right: 0.5rem; }
	.info { color: #888; font-size: 0.85rem; }
	a { color: #53a8b6; text-decoration: none; }
	a:hover { text-decoration: underline; }
</style>
</head>
<body>
<h1><span class="status"></span> GoClaw</h1>
<p class="subtitle">Version %s &mdash; %s</p>

<div class="card">
	<h2>Agents</h2>
	<table>
		<thead>
			<tr><th>ID</th><th>Name</th><th>Model</th><th>Workspace</th><th>Sandbox</th></tr>
		</thead>
		<tbody>
			%s
		</tbody>
	</table>
</div>

<div class="card">
	<h2>Quick Links</h2>
	<p><a href="/health">/health</a> &mdash; Health check endpoint</p>
	<p><a href="/metrics">/metrics</a> &mdash; Prometheus metrics</p>
	<p class="info" style="margin-top: 1rem;">WebSocket endpoint: ws://127.0.0.1:18789/ws</p>
</div>
</body>
</html>`
