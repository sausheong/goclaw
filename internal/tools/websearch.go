package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const searchTimeout = 15 * time.Second

// WebSearchTool searches the web and returns results.
// Uses DuckDuckGo's HTML search (no API key required).
type WebSearchTool struct{}

type webSearchInput struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

func (t *WebSearchTool) Name() string { return "web_search" }

func (t *WebSearchTool) Description() string {
	return "Search the web for information. Returns search results with titles, URLs, and snippets. Use this when you need current information, documentation, or to find web resources."
}

func (t *WebSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "The search query"
			},
			"max_results": {
				"type": "integer",
				"description": "Maximum number of results to return (default: 5, max: 10)"
			}
		},
		"required": ["query"]
	}`)
}

func (t *WebSearchTool) Execute(ctx context.Context, input json.RawMessage) (ToolResult, error) {
	var in webSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	if in.Query == "" {
		return ToolResult{Error: "query is required"}, nil
	}

	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > 10 {
		maxResults = 10
	}

	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	results, err := duckDuckGoSearch(ctx, in.Query, maxResults)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("search failed: %v", err)}, nil
	}

	if len(results) == 0 {
		return ToolResult{Output: "No results found for: " + in.Query}, nil
	}

	var sb strings.Builder
	for i, r := range results {
		fmt.Fprintf(&sb, "%d. **%s**\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet)
	}

	return ToolResult{
		Output: sb.String(),
		Metadata: map[string]any{
			"query":       in.Query,
			"num_results": len(results),
		},
	}, nil
}

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

// duckDuckGoSearch performs a search using DuckDuckGo's HTML interface
// and extracts results. No API key required.
func duckDuckGoSearch(ctx context.Context, query string, maxResults int) ([]searchResult, error) {
	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GoClaw/1.0 (AI Agent Gateway)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return nil, err
	}

	return parseDDGResults(string(body), maxResults), nil
}

// parseDDGResults extracts search results from DuckDuckGo's HTML response.
func parseDDGResults(html string, maxResults int) []searchResult {
	var results []searchResult

	// DuckDuckGo HTML results are in <a class="result__a" ...> tags
	// with snippets in <a class="result__snippet" ...> tags
	remaining := html
	for len(results) < maxResults {
		// Find result link
		linkIdx := strings.Index(remaining, `class="result__a"`)
		if linkIdx == -1 {
			break
		}
		remaining = remaining[linkIdx:]

		// Extract href
		href := extractAttr(remaining, "href")
		// Extract title text
		title := extractTagText(remaining, "a")

		// Find snippet
		snippetIdx := strings.Index(remaining, `class="result__snippet"`)
		snippet := ""
		if snippetIdx != -1 {
			snippet = extractTagText(remaining[snippetIdx:], "a")
		}

		// Clean up the URL — DDG wraps URLs in a redirect
		cleanURL := cleanDDGURL(href)

		if cleanURL != "" && title != "" {
			results = append(results, searchResult{
				Title:   cleanHTMLText(title),
				URL:     cleanURL,
				Snippet: cleanHTMLText(snippet),
			})
		}

		// Move past this result
		nextIdx := strings.Index(remaining[1:], `class="result__a"`)
		if nextIdx == -1 {
			break
		}
		remaining = remaining[nextIdx+1:]
	}

	return results
}

// extractAttr extracts the value of the first href attribute found.
func extractAttr(html, attr string) string {
	needle := attr + `="`
	idx := strings.Index(html, needle)
	if idx == -1 {
		return ""
	}
	start := idx + len(needle)
	end := strings.Index(html[start:], `"`)
	if end == -1 {
		return ""
	}
	return html[start : start+end]
}

// extractTagText extracts text content from the first occurrence of a tag.
func extractTagText(html, tag string) string {
	start := strings.Index(html, ">")
	if start == -1 {
		return ""
	}
	start++
	endTag := "</" + tag + ">"
	end := strings.Index(html[start:], endTag)
	if end == -1 {
		return ""
	}
	return html[start : start+end]
}

// cleanDDGURL extracts the actual URL from DuckDuckGo's redirect wrapper.
func cleanDDGURL(rawURL string) string {
	// DDG wraps URLs like: //duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com&rut=...
	if strings.Contains(rawURL, "uddg=") {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return rawURL
		}
		uddg := parsed.Query().Get("uddg")
		if uddg != "" {
			return uddg
		}
	}
	if strings.HasPrefix(rawURL, "//") {
		return "https:" + rawURL
	}
	return rawURL
}

// cleanHTMLText removes HTML tags and decodes common entities.
func cleanHTMLText(s string) string {
	// Strip HTML tags
	var out strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			out.WriteRune(r)
		}
	}
	result := out.String()
	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&quot;", `"`)
	result = strings.ReplaceAll(result, "&#x27;", "'")
	result = strings.ReplaceAll(result, "&nbsp;", " ")
	return strings.TrimSpace(result)
}
