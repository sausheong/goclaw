package memory

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Entry represents a single memory entry stored as a Markdown file.
type Entry struct {
	ID       string // derived from filename
	Title    string
	Content  string
	FilePath string
	ModTime  time.Time
}

// Manager handles persistent memory stored as Markdown files with BM25 search.
type Manager struct {
	baseDir string
	entries map[string]Entry
	index   *BM25Index
	mu      sync.RWMutex
}

// NewManager creates a new memory manager rooted at the given directory.
func NewManager(baseDir string) *Manager {
	return &Manager{
		baseDir: baseDir,
		entries: make(map[string]Entry),
		index:   NewBM25Index(),
	}
}

// Load scans the memory directory and indexes all Markdown files.
func (m *Manager) Load() error {
	entriesDir := filepath.Join(m.baseDir, "entries")
	if err := os.MkdirAll(entriesDir, 0o755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = make(map[string]Entry)
	m.index = NewBM25Index()

	entries, err := os.ReadDir(entriesDir)
	if err != nil {
		return fmt.Errorf("read memory dir: %w", err)
	}

	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".md") {
			continue
		}

		path := filepath.Join(entriesDir, de.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("failed to read memory entry", "path", path, "error", err)
			continue
		}

		info, _ := de.Info()
		modTime := time.Now()
		if info != nil {
			modTime = info.ModTime()
		}

		id := strings.TrimSuffix(de.Name(), ".md")
		content := string(data)

		// Extract title from first heading or filename
		title := id
		if idx := strings.Index(content, "# "); idx >= 0 {
			end := strings.Index(content[idx:], "\n")
			if end > 0 {
				title = strings.TrimPrefix(content[idx:idx+end], "# ")
			}
		}

		entry := Entry{
			ID:       id,
			Title:    title,
			Content:  content,
			FilePath: path,
			ModTime:  modTime,
		}

		m.entries[id] = entry
		m.index.Add(id, content)
	}

	slog.Info("loaded memory entries", "count", len(m.entries))
	return nil
}

// Save writes a memory entry to disk and updates the index.
func (m *Manager) Save(id, content string) error {
	entriesDir := filepath.Join(m.baseDir, "entries")
	if err := os.MkdirAll(entriesDir, 0o755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}

	path := filepath.Join(entriesDir, id+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write memory entry: %w", err)
	}

	// Extract title
	title := id
	if idx := strings.Index(content, "# "); idx >= 0 {
		end := strings.Index(content[idx:], "\n")
		if end > 0 {
			title = strings.TrimPrefix(content[idx:idx+end], "# ")
		}
	}

	entry := Entry{
		ID:       id,
		Title:    title,
		Content:  content,
		FilePath: path,
		ModTime:  time.Now(),
	}

	m.mu.Lock()
	m.entries[id] = entry
	// Rebuild index (simple approach; for large datasets we'd do incremental updates)
	m.index = NewBM25Index()
	for _, e := range m.entries {
		m.index.Add(e.ID, e.Content)
	}
	m.mu.Unlock()

	return nil
}

// Search queries the memory using BM25 and returns relevant entries.
func (m *Manager) Search(query string, maxResults int) []Entry {
	if maxResults <= 0 {
		maxResults = 5
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	results := m.index.Search(query, maxResults)

	var entries []Entry
	for _, r := range results {
		if e, ok := m.entries[r.ID]; ok {
			entries = append(entries, e)
		}
	}

	return entries
}

// Entries returns all memory entries.
func (m *Manager) Entries() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := make([]Entry, 0, len(m.entries))
	for _, e := range m.entries {
		entries = append(entries, e)
	}
	return entries
}

// Get returns a specific memory entry by ID.
func (m *Manager) Get(id string) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.entries[id]
	return e, ok
}

// Delete removes a memory entry.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[id]
	if !ok {
		return fmt.Errorf("memory entry not found: %s", id)
	}

	if err := os.Remove(entry.FilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete memory file: %w", err)
	}

	delete(m.entries, id)

	// Rebuild index
	m.index = NewBM25Index()
	for _, e := range m.entries {
		m.index.Add(e.ID, e.Content)
	}

	return nil
}

// FormatForPrompt formats relevant memory entries for injection into the system prompt.
func FormatForPrompt(entries []Entry) string {
	if len(entries) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n## Relevant Memory\n\n")

	for _, e := range entries {
		b.WriteString("### ")
		b.WriteString(e.Title)
		b.WriteString("\n\n")
		// Truncate long entries for prompt context
		content := e.Content
		if len(content) > 2000 {
			content = content[:2000] + "\n\n[truncated]"
		}
		b.WriteString(content)
		b.WriteString("\n\n")
	}

	return b.String()
}
