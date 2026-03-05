package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

const (
	maxFetchSize    = 5 * 1024 * 1024 // 5MB
	fetchTimeout    = 30 * time.Second
	maxOutputLength = 50000 // truncate very long pages
)

// WebFetchTool fetches a URL and returns its content as markdown.
type WebFetchTool struct{}

type webFetchInput struct {
	URL     string `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (t *WebFetchTool) Name() string { return "web_fetch" }

func (t *WebFetchTool) Description() string {
	return "Fetch the content of a web page at the given URL. Returns the page content converted to readable markdown text. Use this to read documentation, articles, API responses, or any web content."
}

func (t *WebFetchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "The URL to fetch (must start with http:// or https://)"
			},
			"headers": {
				"type": "object",
				"description": "Optional HTTP headers to include in the request",
				"additionalProperties": { "type": "string" }
			}
		},
		"required": ["url"]
	}`)
}

func (t *WebFetchTool) Execute(ctx context.Context, input json.RawMessage) (ToolResult, error) {
	var in webFetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	if in.URL == "" {
		return ToolResult{Error: "url is required"}, nil
	}
	if !strings.HasPrefix(in.URL, "http://") && !strings.HasPrefix(in.URL, "https://") {
		return ToolResult{Error: "url must start with http:// or https://"}, nil
	}

	if err := validateURLNotInternal(in.URL); err != nil {
		return ToolResult{Error: err.Error()}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.URL, nil)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid URL: %v", err)}, nil
	}

	req.Header.Set("User-Agent", "GoClaw/1.0 (AI Agent Gateway)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,text/plain;q=0.8,application/json;q=0.7,*/*;q=0.5")
	for k, v := range in.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects (max 10)")
			}
			// Re-validate each redirect destination against SSRF blocklist
			if err := validateURLNotInternal(req.URL.String()); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("fetch failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ToolResult{Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status)}, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchSize))
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("read body failed: %v", err)}, nil
	}

	contentType := resp.Header.Get("Content-Type")
	content := string(body)

	// Convert HTML to markdown for readability
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml") {
		md, err := htmltomarkdown.ConvertString(content)
		if err == nil {
			content = md
		}
	}

	// Truncate very long content
	if len(content) > maxOutputLength {
		content = content[:maxOutputLength] + "\n\n[Content truncated — exceeded maximum length]"
	}

	return ToolResult{
		Output: content,
		Metadata: map[string]any{
			"url":          in.URL,
			"status":       resp.StatusCode,
			"content_type": contentType,
			"length":       len(body),
		},
	}, nil
}
