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
#theme-btn:hover { border-color: var(--accent); }
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
.msg.assistant .content ul, .msg.assistant .content ol {
	margin: 0.5em 0 0.5em 1.5em;
}
.msg.assistant .content li { margin-bottom: 0.25em; }
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
</style>
</head>
<body>
<div id="header">
	<img class="logo" src="data:image/png;base64,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" alt="GoClaw">
	<h1>GoClaw</h1>
	<span class="spacer"></span>
	<button id="theme-btn" title="Toggle light/dark mode">&#9790;</button>
	<span class="status" id="conn-status">connecting...</span>
</div>
<div id="messages"></div>
<div id="input-area">
	<textarea id="input" rows="1" placeholder="Type a message..." autofocus></textarea>
	<button id="send-btn" disabled>Send</button>
</div>

<script>
(function() {
	var PORT = %d;
	var messagesEl = document.getElementById('messages');
	var inputEl = document.getElementById('input');
	var sendBtn = document.getElementById('send-btn');
	var connStatus = document.getElementById('conn-status');
	var themeBtn = document.getElementById('theme-btn');

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

	var ws = null;
	var msgId = 0;
	var currentAssistant = null;
	var sending = false;
	var reconnectTimer = null;

	// Simple markdown renderer (no CDN)
	function renderMd(text) {
		var html = text;
		// Code blocks
		html = html.replace(/` + "```" + `(\\w*)\n([\s\S]*?)` + "```" + `/g, function(_, lang, code) {
			return '<pre><code>' + escHtml(code.trimEnd()) + '</code></pre>';
		});
		// Inline code
		html = html.replace(/` + "`" + `([^` + "`" + `]+)` + "`" + `/g, '<code>$1</code>');
		// Bold
		html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
		// Italic
		html = html.replace(/\*(.+?)\*/g, '<em>$1</em>');
		// Links
		html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
		// Paragraphs (split by double newlines)
		html = html.replace(/\n\n+/g, '</p><p>');
		// Single newlines to <br>
		html = html.replace(/\n/g, '<br>');
		return '<p>' + html + '</p>';
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
		sendBtn.disabled = sending || !ws || ws.readyState !== WebSocket.OPEN;
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
			params: { agentId: 'default', text: text },
			id: msgId
		}));

		inputEl.value = '';
		inputEl.style.height = 'auto';
	}

	sendBtn.addEventListener('click', sendMessage);

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
