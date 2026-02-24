package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// Store handles JSONL file I/O for sessions.
type Store struct {
	baseDir string
	mu      sync.Mutex
}

// NewStore creates a new session store.
func NewStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

// sessionDir returns the directory for a given agent's sessions.
func (s *Store) sessionDir(agentID string) string {
	return filepath.Join(s.baseDir, agentID)
}

// sessionPath returns the file path for a session.
func (s *Store) sessionPath(agentID, key string) string {
	return filepath.Join(s.sessionDir(agentID), key+".jsonl")
}

// Load reads a session from its JSONL file.
func (s *Store) Load(agentID, key string) (*Session, error) {
	path := s.sessionPath(agentID, key)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			sess := NewSession(agentID, key)
			sess.SetStore(s)
			return sess, nil
		}
		return nil, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	sess := NewSession(agentID, key)
	sess.SetStore(s)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry SessionEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			slog.Warn("skipping malformed session entry", "error", err)
			continue
		}

		// Add to session without re-persisting
		sess.entries = append(sess.entries, entry)
		sess.entryMap[entry.ID] = &sess.entries[len(sess.entries)-1]
		sess.leafID = entry.ID
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read session file: %w", err)
	}

	return sess, nil
}

// AppendEntry writes a single entry to the session's JSONL file.
func (s *Store) AppendEntry(sess *Session, entry SessionEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.sessionDir(sess.AgentID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Error("failed to create session dir", "error", err)
		return
	}

	path := s.sessionPath(sess.AgentID, sess.Key)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		slog.Error("failed to open session file", "error", err)
		return
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		slog.Error("failed to marshal session entry", "error", err)
		return
	}

	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		slog.Error("failed to write session entry", "error", err)
	}
}

// Rewrite replaces the entire session JSONL file with the current entries.
// Used after compaction to replace the old file.
func (s *Store) Rewrite(sess *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.sessionDir(sess.AgentID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Error("failed to create session dir", "error", err)
		return
	}

	path := s.sessionPath(sess.AgentID, sess.Key)

	f, err := os.Create(path)
	if err != nil {
		slog.Error("failed to create session file for rewrite", "error", err)
		return
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, entry := range sess.Entries() {
		data, err := json.Marshal(entry)
		if err != nil {
			slog.Error("failed to marshal session entry", "error", err)
			continue
		}
		w.Write(data)
		w.WriteByte('\n')
	}

	if err := w.Flush(); err != nil {
		slog.Error("failed to flush session file", "error", err)
	}
}
