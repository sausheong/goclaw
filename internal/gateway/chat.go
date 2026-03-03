package gateway

import (
	"fmt"
	"net/http"
)

// NewChatHandler returns an HTTP handler func that serves the chat web interface.
func NewChatHandler(port int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, chatHTML, port)
	}
}

const chatHTML = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>GoClaw Chat</title>
<style>
:root {
	--bg: #1a1a2e;
	--bg-header: #16213e;
	--bg-msg-user: #0f3460;
	--bg-msg-asst: #16213e;
	--bg-code: #0d1b36;
	--bg-input: #0d1b36;
	--border: #0f3460;
	--text: #e0e0e0;
	--text-muted: #888;
	--text-strong: #fff;
	--text-em: #ccc;
	--accent: #16dbaa;
	--accent2: #53a8b6;
	--btn-text: #1a1a2e;
	--placeholder: #555;
	--error: #e74c3c;
	--tool-output: #aaa;
}
html.light {
	--bg: #f5f5f5;
	--bg-header: #ffffff;
	--bg-msg-user: #d1e7ff;
	--bg-msg-asst: #ffffff;
	--bg-code: #f0f0f0;
	--bg-input: #ffffff;
	--border: #ddd;
	--text: #1a1a1a;
	--text-muted: #777;
	--text-strong: #000;
	--text-em: #333;
	--accent: #0fa888;
	--accent2: #3a7f8c;
	--btn-text: #fff;
	--placeholder: #999;
	--error: #d32f2f;
	--tool-output: #555;
}
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
	font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, monospace;
	background: var(--bg);
	color: var(--text);
	height: 100vh;
	display: flex;
	flex-direction: column;
	transition: background 0.3s, color 0.3s;
}
#header {
	background: var(--bg-header);
	padding: 0.75rem 1.5rem;
	border-bottom: 1px solid var(--border);
	display: flex;
	align-items: center;
	gap: 0.75rem;
	flex-shrink: 0;
	transition: background 0.3s, border-color 0.3s;
}
#header .logo {
	width: 24px; height: 24px;
	filter: invert(1);
	transition: filter 0.3s;
}
html.light #header .logo {
	filter: none;
}
#header h1 { font-size: 1.1rem; color: var(--accent); }
#header .status { font-size: 0.8rem; color: var(--text-muted); }
#header .spacer { margin-left: auto; }
#theme-btn {
	background: none;
	border: 1px solid var(--border);
	border-radius: 6px;
	padding: 0.3rem 0.5rem;
	cursor: pointer;
	font-size: 1rem;
	line-height: 1;
	color: var(--text);
	transition: border-color 0.3s;
}
#theme-btn:hover, #clear-btn:hover, #agent-select:hover { border-color: var(--accent); }
#agent-select {
	background: var(--bg-input);
	border: 1px solid var(--border);
	border-radius: 6px;
	padding: 0.3rem 0.5rem;
	font-size: 0.85rem;
	color: var(--text);
	font-family: inherit;
	outline: none;
	cursor: pointer;
	transition: background 0.3s, border-color 0.3s, color 0.3s;
}
#agent-select:focus { border-color: var(--accent); }
#clear-btn {
	background: none;
	border: 1px solid var(--border);
	border-radius: 6px;
	padding: 0.3rem 0.5rem;
	cursor: pointer;
	font-size: 0.8rem;
	line-height: 1;
	color: var(--text);
	transition: border-color 0.3s;
}
#messages {
	flex: 1;
	overflow-y: auto;
	padding: 1rem 1.5rem;
	display: flex;
	flex-direction: column;
	gap: 1rem;
}
.msg {
	max-width: 85%%;
	padding: 0.75rem 1rem;
	border-radius: 12px;
	line-height: 1.5;
	word-wrap: break-word;
	overflow-wrap: break-word;
	transition: background 0.3s, border-color 0.3s;
}
.msg.user {
	background: var(--bg-msg-user);
	align-self: flex-end;
	border-bottom-right-radius: 4px;
}
.msg.assistant {
	background: var(--bg-msg-asst);
	align-self: flex-start;
	border-bottom-left-radius: 4px;
	border: 1px solid var(--border);
}
.msg.assistant .content p { margin-bottom: 0.5em; }
.msg.assistant .content p:last-child { margin-bottom: 0; }
.msg.assistant .content code {
	background: var(--bg-code);
	padding: 0.15em 0.4em;
	border-radius: 3px;
	font-size: 0.9em;
	font-family: "SF Mono", "Fira Code", monospace;
}
.msg.assistant .content pre {
	background: var(--bg-code);
	padding: 0.75rem;
	border-radius: 6px;
	overflow-x: auto;
	margin: 0.5em 0;
	border: 1px solid var(--border);
	transition: background 0.3s, border-color 0.3s;
}
.msg.assistant .content pre code {
	background: none;
	padding: 0;
	font-size: 0.85em;
}
.msg.assistant .content a { color: var(--accent2); }
.msg.assistant .content strong { color: var(--text-strong); }
.msg.assistant .content em { color: var(--text-em); }
.msg.assistant .content h1,
.msg.assistant .content h2,
.msg.assistant .content h3,
.msg.assistant .content h4,
.msg.assistant .content h5,
.msg.assistant .content h6 {
	margin: 0.75em 0 0.25em;
	color: var(--text-strong);
}
.msg.assistant .content h1 { font-size: 1.4em; }
.msg.assistant .content h2 { font-size: 1.2em; }
.msg.assistant .content h3 { font-size: 1.05em; }
.msg.assistant .content hr {
	border: none;
	border-top: 1px solid var(--border);
	margin: 0.75em 0;
}
.msg.assistant .content ul, .msg.assistant .content ol {
	margin: 0.5em 0 0.5em 1.5em;
}
.msg.assistant .content li { margin-bottom: 0.25em; }
.msg.assistant .content table {
	border-collapse: collapse;
	margin: 0.5em 0;
	display: block;
	overflow-x: auto;
	max-width: 100%%;
}
.msg.assistant .content th,
.msg.assistant .content td {
	border: 1px solid var(--border);
	padding: 0.4em 0.75em;
	text-align: left;
}
.msg.assistant .content th {
	background: var(--bg-code);
	color: var(--text-strong);
	font-weight: 600;
}
.msg.assistant .content tr:nth-child(even) td {
	background: rgba(128,128,128,0.07);
}
.tool-call {
	background: var(--bg-code);
	border: 1px solid var(--border);
	border-radius: 6px;
	margin: 0.5rem 0;
	font-size: 0.85rem;
	max-width: 85%%;
	align-self: flex-start;
	transition: background 0.3s, border-color 0.3s;
}
.tool-call-header {
	padding: 0.4rem 0.75rem;
	color: var(--accent2);
	cursor: pointer;
	display: flex;
	align-items: center;
	gap: 0.5rem;
	user-select: none;
}
.tool-call-header .arrow {
	font-size: 0.7em;
	transition: transform 0.2s;
}
.tool-call-header .arrow.open { transform: rotate(90deg); }
.tool-call-output {
	display: none;
	padding: 0.5rem 0.75rem;
	border-top: 1px solid var(--border);
	color: var(--tool-output);
	white-space: pre-wrap;
	max-height: 300px;
	overflow-y: auto;
	font-family: "SF Mono", "Fira Code", monospace;
	font-size: 0.8rem;
}
.tool-call-output.show { display: block; }
.tool-call-output.error { color: var(--error); }
.tool-call-header .tool-detail {
	color: var(--text-muted);
	font-family: "SF Mono", "Fira Code", monospace;
	font-size: 0.9em;
	max-width: 500px;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
	display: inline-block;
	vertical-align: bottom;
}
#input-area {
	background: var(--bg-header);
	padding: 0.75rem 1.5rem;
	border-top: 1px solid var(--border);
	display: flex;
	gap: 0.75rem;
	flex-shrink: 0;
	transition: background 0.3s, border-color 0.3s;
}
#input {
	flex: 1;
	background: var(--bg-input);
	border: 1px solid var(--border);
	border-radius: 8px;
	padding: 0.6rem 1rem;
	color: var(--text);
	font-size: 0.95rem;
	font-family: inherit;
	outline: none;
	resize: none;
	min-height: 40px;
	max-height: 150px;
	transition: background 0.3s, border-color 0.3s, color 0.3s;
}
#input:focus { border-color: var(--accent); }
#input::placeholder { color: var(--placeholder); }
#send-btn {
	background: var(--accent);
	color: var(--btn-text);
	border: none;
	border-radius: 8px;
	padding: 0 1.25rem;
	font-size: 0.95rem;
	font-weight: 600;
	cursor: pointer;
	transition: opacity 0.2s, background 0.3s;
	align-self: flex-end;
	height: 40px;
}
#send-btn:hover { opacity: 0.85; }
#send-btn:disabled { opacity: 0.4; cursor: not-allowed; }
#stop-btn {
	background: var(--error);
	color: #fff;
	border: none;
	border-radius: 8px;
	padding: 0 1.25rem;
	font-size: 0.95rem;
	font-weight: 600;
	cursor: pointer;
	transition: opacity 0.2s, background 0.3s;
	align-self: flex-end;
	height: 40px;
	display: none;
}
#stop-btn:hover { opacity: 0.85; }
</style>
</head>
<body>
<div id="header">
	<h1>GoClaw</h1>
	<select id="agent-select" title="Select agent"></select>
	<span class="spacer"></span>
	<button id="clear-btn" title="Clear session">Clear</button>
	<button id="theme-btn" title="Toggle light/dark mode">&#9790;</button>
	<span class="status" id="conn-status">connecting...</span>
</div>
<div id="messages"></div>
<div id="input-area">
	<textarea id="input" rows="1" placeholder="Type a message..." autofocus></textarea>
	<button id="send-btn" disabled>Send</button>
	<button id="stop-btn">Stop</button>
</div>

<script>
(function() {
	var PORT = %d;
	var messagesEl = document.getElementById('messages');
	var inputEl = document.getElementById('input');
	var sendBtn = document.getElementById('send-btn');
	var connStatus = document.getElementById('conn-status');
	var themeBtn = document.getElementById('theme-btn');
	var clearBtn = document.getElementById('clear-btn');
	var stopBtn = document.getElementById('stop-btn');
	var agentSelect = document.getElementById('agent-select');

	// Theme toggle
	function setTheme(mode) {
		if (mode === 'light') {
			document.documentElement.classList.add('light');
			themeBtn.innerHTML = '&#9728;';
			themeBtn.title = 'Switch to dark mode';
		} else {
			document.documentElement.classList.remove('light');
			themeBtn.innerHTML = '&#9790;';
			themeBtn.title = 'Switch to light mode';
		}
		localStorage.setItem('goclaw-theme', mode);
	}

	var saved = localStorage.getItem('goclaw-theme') || 'dark';
	setTheme(saved);

	themeBtn.addEventListener('click', function() {
		var current = document.documentElement.classList.contains('light') ? 'light' : 'dark';
		setTheme(current === 'light' ? 'dark' : 'light');
	});

	clearBtn.addEventListener('click', function() {
		if (!ws || ws.readyState !== WebSocket.OPEN) return;
		ws.send(JSON.stringify({
			jsonrpc: '2.0',
			method: 'session.clear',
			params: { agentId: agentSelect.value },
			id: 'clear'
		}));
		messagesEl.innerHTML = '';
		currentAssistant = null;
		toolEls = {};
	});

	agentSelect.addEventListener('change', function() {
		messagesEl.innerHTML = '';
		currentAssistant = null;
		toolEls = {};
		if (!ws || ws.readyState !== WebSocket.OPEN) return;
		ws.send(JSON.stringify({
			jsonrpc: '2.0',
			method: 'session.history',
			params: { agentId: agentSelect.value },
			id: 'history'
		}));
	});

	var ws = null;
	var msgId = 0;
	var currentAssistant = null;
	var sending = false;
	var reconnectTimer = null;

	// Inline markdown: code, bold, italic, links
	function inlineMd(s) {
		s = s.replace(/` + "`" + `([^` + "`" + `]+)` + "`" + `/g, function(_, code) {
			return '<code>' + escHtml(code) + '</code>';
		});
		s = s.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
		s = s.replace(/\*(.+?)\*/g, '<em>$1</em>');
		s = s.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
		return s;
	}

	// Block-level markdown renderer
	function renderMd(text) {
		// 1. Extract code blocks into placeholders
		var codeBlocks = [];
		var s = text.replace(/` + "```" + `(\\w*)\n([\\s\\S]*?)` + "```" + `/g, function(_, lang, code) {
			var idx = codeBlocks.length;
			codeBlocks.push('<pre><code>' + escHtml(code.trimEnd()) + '</code></pre>');
			return '__CB' + idx + '__';
		});

		// 2. Process lines
		var lines = s.split('\n');
		var html = '';
		var inUl = false, inOl = false, inP = false;

		function closeAll() {
			if (inP) { html += '</p>'; inP = false; }
			if (inUl) { html += '</ul>'; inUl = false; }
			if (inOl) { html += '</ol>'; inOl = false; }
		}

		for (var i = 0; i < lines.length; i++) {
			var t = lines[i].trim();

			// Code block placeholder
			if (/^__CB\d+__$/.test(t)) {
				closeAll();
				html += t;
				continue;
			}

			// Empty line — close paragraphs but keep lists open
			// (loose list items separated by blank lines stay in the same list)
			if (t === '') {
				if (inP) { html += '</p>'; inP = false; }
				continue;
			}

			// Horizontal rule (before list check so --- isn't a list item)
			if (/^[-*_]{3,}$/.test(t)) {
				closeAll();
				html += '<hr>';
				continue;
			}

			// Heading
			var hm = t.match(/^(#{1,6})\s+(.*)$/);
			if (hm) {
				closeAll();
				var lvl = hm[1].length;
				html += '<h' + lvl + '>' + inlineMd(hm[2]) + '</h' + lvl + '>';
				continue;
			}

			// Unordered list item
			var um = t.match(/^[-*]\s+(.*)$/);
			if (um) {
				if (inP) { html += '</p>'; inP = false; }
				if (inOl) { html += '</ol>'; inOl = false; }
				if (!inUl) { html += '<ul>'; inUl = true; }
				html += '<li>' + inlineMd(um[1]) + '</li>';
				continue;
			}

			// Ordered list item
			var om = t.match(/^\d+[.)]\s+(.*)$/);
			if (om) {
				if (inP) { html += '</p>'; inP = false; }
				if (inUl) { html += '</ul>'; inUl = false; }
				if (!inOl) { html += '<ol>'; inOl = true; }
				html += '<li>' + inlineMd(om[1]) + '</li>';
				continue;
			}

			// Table: line starts with | and next line is a separator row
			if (t.charAt(0) === '|' && i + 1 < lines.length) {
				var sepLine = lines[i + 1].trim();
				if (/^\|[\s\-:]+(\|[\s\-:]+)+\|?\s*$/.test(sepLine)) {
					closeAll();
					// Parse alignment from separator
					var sepCells = sepLine.replace(/^\||\|$/g, '').split('|');
					var aligns = [];
					for (var a = 0; a < sepCells.length; a++) {
						var sc = sepCells[a].trim();
						if (sc.charAt(0) === ':' && sc.charAt(sc.length - 1) === ':') aligns.push('center');
						else if (sc.charAt(sc.length - 1) === ':') aligns.push('right');
						else aligns.push('left');
					}
					// Parse header row
					var hdrs = t.replace(/^\||\|$/g, '').split('|');
					var tbl = '<table><thead><tr>';
					for (var h = 0; h < hdrs.length; h++) {
						var al = aligns[h] || 'left';
						tbl += '<th style="text-align:' + al + '">' + inlineMd(hdrs[h].trim()) + '</th>';
					}
					tbl += '</tr></thead><tbody>';
					// Skip separator line
					i += 2;
					// Parse body rows
					while (i < lines.length && lines[i].trim().charAt(0) === '|') {
						var cells = lines[i].trim().replace(/^\||\|$/g, '').split('|');
						tbl += '<tr>';
						for (var c = 0; c < cells.length; c++) {
							var cal = aligns[c] || 'left';
							tbl += '<td style="text-align:' + cal + '">' + inlineMd(cells[c].trim()) + '</td>';
						}
						tbl += '</tr>';
						i++;
					}
					tbl += '</tbody></table>';
					html += tbl;
					i--; // compensate for loop increment
					continue;
				}
			}

			// Regular text — close any open list first
			if (inUl) { html += '</ul>'; inUl = false; }
			if (inOl) { html += '</ol>'; inOl = false; }
			if (inP) {
				html += '<br>' + inlineMd(t);
			} else {
				html += '<p>' + inlineMd(t);
				inP = true;
			}
		}

		if (inP) html += '</p>';
		if (inUl) html += '</ul>';
		if (inOl) html += '</ol>';

		// 3. Restore code blocks
		for (var i = 0; i < codeBlocks.length; i++) {
			html = html.replace('__CB' + i + '__', codeBlocks[i]);
		}

		return html;
	}

	function escHtml(s) {
		return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
	}

	function scrollToBottom() {
		messagesEl.scrollTop = messagesEl.scrollHeight;
	}

	function connect() {
		ws = new WebSocket('ws://localhost:' + PORT + '/ws');

		ws.onopen = function() {
			connStatus.textContent = 'connected';
			sendBtn.disabled = false;
			if (reconnectTimer) {
				clearTimeout(reconnectTimer);
				reconnectTimer = null;
			}
			// Fetch available agents first, then load history
			ws.send(JSON.stringify({
				jsonrpc: '2.0',
				method: 'agent.status',
				params: {},
				id: 'agents'
			}));
		};

		ws.onclose = function() {
			connStatus.textContent = 'disconnected';
			sendBtn.disabled = true;
			sending = false;
			reconnectTimer = setTimeout(connect, 3000);
		};

		ws.onerror = function() {
			connStatus.textContent = 'error';
		};

		ws.onmessage = function(e) {
			try {
				var resp = JSON.parse(e.data);
				if (resp.error) {
					addError(typeof resp.error === 'string' ? resp.error : resp.error.message || JSON.stringify(resp.error));
					sending = false;
					updateSendBtn();
					return;
				}
				if (!resp.result) return;

				// Handle agent.status response
				if (resp.id === 'agents') {
					var agents = resp.result.agents || [];
					agentSelect.innerHTML = '';
					for (var i = 0; i < agents.length; i++) {
						var opt = document.createElement('option');
						opt.value = agents[i].id;
						opt.textContent = agents[i].name || agents[i].id;
						agentSelect.appendChild(opt);
					}
					if (agents.length === 0) {
						var opt = document.createElement('option');
						opt.value = 'default';
						opt.textContent = 'default';
						agentSelect.appendChild(opt);
					}
					// Load session history for the selected agent
					ws.send(JSON.stringify({
						jsonrpc: '2.0',
						method: 'session.history',
						params: { agentId: agentSelect.value },
						id: 'history'
					}));
					return;
				}

				// Handle history response
				if (resp.id === 'history') {
					var entries = resp.result.entries || [];
					for (var i = 0; i < entries.length; i++) {
						var entry = entries[i];
						if (entry.type === 'message' && entry.role === 'user') {
							addUserMsg(entry.text);
						} else if (entry.type === 'message' && entry.role === 'assistant') {
							var bubble = addAssistantMsg();
							bubble.raw = entry.text;
							bubble.content.innerHTML = renderMd(entry.text);
						} else if (entry.type === 'tool_call') {
							addToolCall(entry.tool, entry.id, entry.input);
						} else if (entry.type === 'tool_result') {
							updateToolResult(null, entry.tool_call_id, null, entry.output, entry.error);
						}
					}
					scrollToBottom();
					return;
				}

				var r = resp.result;

				switch (r.type) {
				case 'text_delta':
					if (!currentAssistant) {
						currentAssistant = addAssistantMsg('');
					}
					appendToAssistant(r.text);
					break;
				case 'tool_call_start':
					if (currentAssistant) {
						finalizeAssistant();
						currentAssistant = null;
					}
					addToolCall(r.tool, r.id, r.input);
					break;
				case 'tool_result':
					updateToolResult(r.tool, r.id, r.input, r.output, r.error);
					break;
				case 'done':
					if (currentAssistant) {
						finalizeAssistant();
					}
					currentAssistant = null;
					sending = false;
					updateSendBtn();
					break;
				case 'aborted':
					if (currentAssistant) {
						finalizeAssistant();
					}
					currentAssistant = null;
					sending = false;
					updateSendBtn();
					break;
				case 'error':
					addError(r.message);
					currentAssistant = null;
					sending = false;
					updateSendBtn();
					break;
				}
			} catch(err) {
				console.error('parse error:', err);
			}
		};
	}

	function addUserMsg(text) {
		var div = document.createElement('div');
		div.className = 'msg user';
		div.textContent = text;
		messagesEl.appendChild(div);
		scrollToBottom();
	}

	function addAssistantMsg() {
		var div = document.createElement('div');
		div.className = 'msg assistant';
		var content = document.createElement('div');
		content.className = 'content';
		div.appendChild(content);
		messagesEl.appendChild(div);
		scrollToBottom();
		return { el: div, content: content, raw: '' };
	}

	function appendToAssistant(text) {
		if (!currentAssistant) return;
		currentAssistant.raw += text;
		currentAssistant.content.innerHTML = renderMd(currentAssistant.raw);
		scrollToBottom();
	}

	function finalizeAssistant() {
		if (!currentAssistant) return;
		currentAssistant.content.innerHTML = renderMd(currentAssistant.raw);
		scrollToBottom();
	}

	var toolEls = {};

	function toolSummary(toolName, input) {
		if (!input) return escHtml(toolName);
		try {
			var p = (typeof input === 'string') ? JSON.parse(input) : input;
			switch (toolName) {
			case 'bash':
				if (p.command) return escHtml(toolName) + ': <span class="tool-detail">' + escHtml(p.command) + '</span>';
				break;
			case 'read_file':
				if (p.path) return escHtml(toolName) + ': <span class="tool-detail">' + escHtml(p.path) + '</span>';
				break;
			case 'write_file':
				if (p.path) return escHtml(toolName) + ': <span class="tool-detail">' + escHtml(p.path) + '</span>';
				break;
			case 'edit_file':
				if (p.path) return escHtml(toolName) + ': <span class="tool-detail">' + escHtml(p.path) + '</span>';
				break;
			case 'web_fetch':
				if (p.url) return escHtml(toolName) + ': <span class="tool-detail">' + escHtml(p.url) + '</span>';
				break;
			case 'web_search':
				if (p.query) return escHtml(toolName) + ': <span class="tool-detail">' + escHtml(p.query) + '</span>';
				break;
			case 'browser':
				if (p.action) {
					var detail = p.action;
					if (p.url) detail += ' ' + p.url;
					else if (p.selector) detail += ' ' + p.selector;
					return escHtml(toolName) + ': <span class="tool-detail">' + escHtml(detail) + '</span>';
				}
				break;
			case 'send_message':
				if (p.channel) return escHtml(toolName) + ': <span class="tool-detail">' + escHtml(p.channel + ' → ' + (p.chat_id || '')) + '</span>';
				break;
			case 'cron':
				if (p.action) return escHtml(toolName) + ': <span class="tool-detail">' + escHtml(p.action + (p.name ? ' ' + p.name : '')) + '</span>';
				break;
			}
		} catch(e) {}
		return escHtml(toolName);
	}

	function addToolCall(toolName, toolId, input) {
		var div = document.createElement('div');
		div.className = 'tool-call';
		var id = toolId || toolName;
		div.dataset.toolId = id;

		var header = document.createElement('div');
		header.className = 'tool-call-header';
		header.innerHTML = '<span class="arrow">&#9654;</span> ' + toolSummary(toolName, input);
		header.onclick = function() {
			var arrow = header.querySelector('.arrow');
			var output = div.querySelector('.tool-call-output');
			if (output) {
				output.classList.toggle('show');
				arrow.classList.toggle('open');
			}
		};

		var output = document.createElement('div');
		output.className = 'tool-call-output';

		div.appendChild(header);
		div.appendChild(output);
		messagesEl.appendChild(div);
		toolEls[id] = div;
		scrollToBottom();
	}

	function updateToolResult(toolName, toolId, input, outputText, errorText) {
		var el = toolEls[toolId] || toolEls[toolName];
		if (!el) return;

		// Update header with input if we now have it
		if (input) {
			var header = el.querySelector('.tool-call-header');
			if (header) {
				var arrow = header.querySelector('.arrow');
				var isOpen = arrow && arrow.classList.contains('open');
				header.innerHTML = '<span class="arrow' + (isOpen ? ' open' : '') + '">&#9654;</span> ' + toolSummary(toolName, input);
				header.onclick = function() {
					var a = header.querySelector('.arrow');
					var o = el.querySelector('.tool-call-output');
					if (o) { o.classList.toggle('show'); a.classList.toggle('open'); }
				};
			}
		}

		var output = el.querySelector('.tool-call-output');
		if (!output) return;
		if (errorText) {
			output.textContent = errorText;
			output.classList.add('error');
		} else if (outputText) {
			var display = outputText.length > 2000 ? outputText.substring(0, 2000) + '\n...(truncated)' : outputText;
			output.textContent = display;
		} else {
			output.textContent = '(no output)';
		}
	}

	function addError(msg) {
		var div = document.createElement('div');
		div.className = 'msg assistant';
		div.style.borderColor = 'var(--error)';
		div.innerHTML = '<div class="content" style="color:var(--error)">' + escHtml(msg) + '</div>';
		messagesEl.appendChild(div);
		scrollToBottom();
	}

	function updateSendBtn() {
		if (sending) {
			sendBtn.style.display = 'none';
			stopBtn.style.display = 'block';
		} else {
			sendBtn.style.display = 'block';
			stopBtn.style.display = 'none';
			sendBtn.disabled = !ws || ws.readyState !== WebSocket.OPEN;
		}
	}

	function sendMessage() {
		var text = inputEl.value.trim();
		if (!text || sending) return;
		if (!ws || ws.readyState !== WebSocket.OPEN) return;

		addUserMsg(text);
		sending = true;
		updateSendBtn();
		msgId++;

		ws.send(JSON.stringify({
			jsonrpc: '2.0',
			method: 'chat.send',
			params: { agentId: agentSelect.value, text: text },
			id: msgId
		}));

		inputEl.value = '';
		inputEl.style.height = 'auto';
	}

	sendBtn.addEventListener('click', sendMessage);

	stopBtn.addEventListener('click', function() {
		if (!ws || ws.readyState !== WebSocket.OPEN) return;
		ws.send(JSON.stringify({
			jsonrpc: '2.0',
			method: 'chat.abort',
			params: {},
			id: 'abort'
		}));
	});

	inputEl.addEventListener('keydown', function(e) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			sendMessage();
		}
	});

	// Auto-resize textarea
	inputEl.addEventListener('input', function() {
		this.style.height = 'auto';
		this.style.height = Math.min(this.scrollHeight, 150) + 'px';
	});

	connect();
})();
</script>
</body>
</html>`
