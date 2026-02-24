package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

const browserTimeout = 60 * time.Second

// BrowserTool provides headless browser automation via Chrome DevTools Protocol.
type BrowserTool struct{}

type browserInput struct {
	Action   string `json:"action"`             // navigate, click, type, screenshot, get_text, evaluate
	URL      string `json:"url,omitempty"`       // for navigate
	Selector string `json:"selector,omitempty"`  // CSS selector for click, type, get_text
	Text     string `json:"text,omitempty"`      // for type action
	Script   string `json:"script,omitempty"`    // for evaluate action
	Timeout  int    `json:"timeout,omitempty"`   // seconds, optional
}

func (t *BrowserTool) Name() string { return "browser" }

func (t *BrowserTool) Description() string {
	return `Control a headless Chrome browser. Supports these actions:
- "navigate": Go to a URL. Requires "url".
- "click": Click an element. Requires "selector" (CSS selector).
- "type": Type text into an input field. Requires "selector" and "text".
- "get_text": Get the text content of an element or the full page. Optional "selector" (defaults to body).
- "screenshot": Take a screenshot of the current page. Returns a base64-encoded PNG.
- "evaluate": Execute JavaScript in the page. Requires "script". Returns the result.
Use this for pages that require JavaScript rendering, form interactions, or visual inspection.`
}

func (t *BrowserTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["navigate", "click", "type", "get_text", "screenshot", "evaluate"],
				"description": "The browser action to perform"
			},
			"url": {
				"type": "string",
				"description": "URL to navigate to (required for navigate action)"
			},
			"selector": {
				"type": "string",
				"description": "CSS selector for the target element (required for click, type; optional for get_text)"
			},
			"text": {
				"type": "string",
				"description": "Text to type (required for type action)"
			},
			"script": {
				"type": "string",
				"description": "JavaScript code to evaluate (required for evaluate action)"
			},
			"timeout": {
				"type": "integer",
				"description": "Timeout in seconds (default: 60)"
			}
		},
		"required": ["action"]
	}`)
}

func (t *BrowserTool) Execute(ctx context.Context, input json.RawMessage) (ToolResult, error) {
	var in browserInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	if in.Action == "" {
		return ToolResult{Error: "action is required"}, nil
	}

	timeout := browserTimeout
	if in.Timeout > 0 {
		timeout = time.Duration(in.Timeout) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create a new browser context for each invocation
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx,
		append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
		)...,
	)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	switch in.Action {
	case "navigate":
		return t.navigate(taskCtx, in)
	case "click":
		return t.click(taskCtx, in)
	case "type":
		return t.typeText(taskCtx, in)
	case "get_text":
		return t.getText(taskCtx, in)
	case "screenshot":
		return t.screenshot(taskCtx, in)
	case "evaluate":
		return t.evaluate(taskCtx, in)
	default:
		return ToolResult{Error: fmt.Sprintf("unknown action: %q (valid: navigate, click, type, get_text, screenshot, evaluate)", in.Action)}, nil
	}
}

func (t *BrowserTool) navigate(ctx context.Context, in browserInput) (ToolResult, error) {
	if in.URL == "" {
		return ToolResult{Error: "url is required for navigate action"}, nil
	}
	if !strings.HasPrefix(in.URL, "http://") && !strings.HasPrefix(in.URL, "https://") {
		return ToolResult{Error: "url must start with http:// or https://"}, nil
	}

	var title string
	err := chromedp.Run(ctx,
		chromedp.Navigate(in.URL),
		chromedp.WaitReady("body"),
		chromedp.Title(&title),
	)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("navigate failed: %v", err)}, nil
	}

	return ToolResult{
		Output: fmt.Sprintf("Navigated to %s\nPage title: %s", in.URL, title),
		Metadata: map[string]any{
			"url":   in.URL,
			"title": title,
		},
	}, nil
}

func (t *BrowserTool) click(ctx context.Context, in browserInput) (ToolResult, error) {
	if in.Selector == "" {
		return ToolResult{Error: "selector is required for click action"}, nil
	}

	err := chromedp.Run(ctx,
		chromedp.WaitVisible(in.Selector),
		chromedp.Click(in.Selector),
	)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("click failed on %q: %v", in.Selector, err)}, nil
	}

	return ToolResult{Output: fmt.Sprintf("Clicked element: %s", in.Selector)}, nil
}

func (t *BrowserTool) typeText(ctx context.Context, in browserInput) (ToolResult, error) {
	if in.Selector == "" {
		return ToolResult{Error: "selector is required for type action"}, nil
	}
	if in.Text == "" {
		return ToolResult{Error: "text is required for type action"}, nil
	}

	err := chromedp.Run(ctx,
		chromedp.WaitVisible(in.Selector),
		chromedp.Clear(in.Selector),
		chromedp.SendKeys(in.Selector, in.Text),
	)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("type failed on %q: %v", in.Selector, err)}, nil
	}

	return ToolResult{Output: fmt.Sprintf("Typed text into element: %s", in.Selector)}, nil
}

func (t *BrowserTool) getText(ctx context.Context, in browserInput) (ToolResult, error) {
	selector := in.Selector
	if selector == "" {
		selector = "body"
	}

	var text string
	err := chromedp.Run(ctx,
		chromedp.WaitReady(selector),
		chromedp.InnerHTML(selector, &text),
	)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("get_text failed on %q: %v", selector, err)}, nil
	}

	// Truncate very long content
	if len(text) > maxOutputLength {
		text = text[:maxOutputLength] + "\n\n[Content truncated]"
	}

	return ToolResult{Output: text}, nil
}

func (t *BrowserTool) screenshot(ctx context.Context, in browserInput) (ToolResult, error) {
	// Navigate first if URL is provided
	if in.URL != "" {
		err := chromedp.Run(ctx,
			chromedp.Navigate(in.URL),
			chromedp.WaitReady("body"),
		)
		if err != nil {
			return ToolResult{Error: fmt.Sprintf("navigate for screenshot failed: %v", err)}, nil
		}
	}

	var buf []byte
	err := chromedp.Run(ctx,
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("screenshot failed: %v", err)}, nil
	}

	encoded := base64.StdEncoding.EncodeToString(buf)

	return ToolResult{
		Output: fmt.Sprintf("Screenshot captured (%d bytes). Base64-encoded PNG data:\n%s", len(buf), encoded),
		Metadata: map[string]any{
			"format": "png",
			"size":   len(buf),
		},
	}, nil
}

func (t *BrowserTool) evaluate(ctx context.Context, in browserInput) (ToolResult, error) {
	if in.Script == "" {
		return ToolResult{Error: "script is required for evaluate action"}, nil
	}

	var result any
	err := chromedp.Run(ctx,
		chromedp.Evaluate(in.Script, &result),
	)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("evaluate failed: %v", err)}, nil
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("%v", result)}, nil
	}

	return ToolResult{Output: string(output)}, nil
}
