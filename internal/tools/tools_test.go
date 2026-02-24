package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	RegisterCoreTools(reg, "")

	names := reg.Names()
	assert.Len(t, names, 7)

	for _, name := range []string{"read_file", "write_file", "edit_file", "bash", "web_fetch", "web_search", "browser"} {
		tool, ok := reg.Get(name)
		assert.True(t, ok, "tool %q should exist", name)
		assert.Equal(t, name, tool.Name())
		assert.NotEmpty(t, tool.Description())
		assert.NotEmpty(t, tool.Parameters())
	}
}

func TestToolDefs(t *testing.T) {
	reg := NewRegistry()
	RegisterCoreTools(reg, "")
	defs := reg.ToolDefs()
	assert.Len(t, defs, 7)
}

func TestReadFileTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	tool := &ReadFileTool{}
	input, _ := json.Marshal(readFileInput{Path: path})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "hello world", result.Output)
	assert.Empty(t, result.Error)
}

func TestReadFileToolMissing(t *testing.T) {
	tool := &ReadFileTool{}
	input, _ := json.Marshal(readFileInput{Path: "/nonexistent/file"})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Error)
}

func TestWriteFileTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "output.txt")

	tool := &WriteFileTool{}
	input, _ := json.Marshal(writeFileInput{Path: path, Content: "test content"})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Output, "Successfully wrote")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(data))
}

func TestEditFileTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	tool := &EditFileTool{}
	input, _ := json.Marshal(editFileInput{
		Path:      path,
		OldString: "world",
		NewString: "Go",
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Empty(t, result.Error)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello Go", string(data))
}

func TestEditFileToolNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	tool := &EditFileTool{}
	input, _ := json.Marshal(editFileInput{
		Path:      path,
		OldString: "missing",
		NewString: "replacement",
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Error, "not found")
}

func TestBashTool(t *testing.T) {
	tool := &BashTool{}
	input, _ := json.Marshal(bashInput{Command: "echo hello"})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", result.Output)
	assert.Empty(t, result.Error)
}

func TestBashToolError(t *testing.T) {
	tool := &BashTool{}
	input, _ := json.Marshal(bashInput{Command: "exit 1"})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Error)
}
