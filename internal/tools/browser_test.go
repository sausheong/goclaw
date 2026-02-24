package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowserToolName(t *testing.T) {
	tool := &BrowserTool{}
	assert.Equal(t, "browser", tool.Name())
}

func TestBrowserToolParameters(t *testing.T) {
	tool := &BrowserTool{}
	params := tool.Parameters()
	assert.True(t, json.Valid(params), "Parameters() should return valid JSON")
}

func TestBrowserToolMissingAction(t *testing.T) {
	tool := &BrowserTool{}
	input, _ := json.Marshal(browserInput{})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Error, "action is required")
}

func TestBrowserToolUnknownAction(t *testing.T) {
	tool := &BrowserTool{}
	input, _ := json.Marshal(browserInput{Action: "fly"})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Error, "unknown action")
	assert.Contains(t, result.Error, "fly")
}

func TestBrowserNavigateMissingURL(t *testing.T) {
	tool := &BrowserTool{}
	result, err := tool.navigate(context.Background(), browserInput{Action: "navigate"})
	require.NoError(t, err)
	assert.Contains(t, result.Error, "url is required")
}

func TestBrowserNavigateInvalidURL(t *testing.T) {
	tool := &BrowserTool{}
	result, err := tool.navigate(context.Background(), browserInput{Action: "navigate", URL: "ftp://example.com"})
	require.NoError(t, err)
	assert.Contains(t, result.Error, "url must start with http")
}

func TestBrowserClickMissingSelector(t *testing.T) {
	tool := &BrowserTool{}
	result, err := tool.click(context.Background(), browserInput{Action: "click"})
	require.NoError(t, err)
	assert.Contains(t, result.Error, "selector is required")
}

func TestBrowserTypeMissingSelector(t *testing.T) {
	tool := &BrowserTool{}
	result, err := tool.typeText(context.Background(), browserInput{Action: "type", Text: "hello"})
	require.NoError(t, err)
	assert.Contains(t, result.Error, "selector is required")
}

func TestBrowserTypeMissingText(t *testing.T) {
	tool := &BrowserTool{}
	result, err := tool.typeText(context.Background(), browserInput{Action: "type", Selector: "#input"})
	require.NoError(t, err)
	assert.Contains(t, result.Error, "text is required")
}

func TestBrowserEvaluateMissingScript(t *testing.T) {
	tool := &BrowserTool{}
	result, err := tool.evaluate(context.Background(), browserInput{Action: "evaluate"})
	require.NoError(t, err)
	assert.Contains(t, result.Error, "script is required")
}
