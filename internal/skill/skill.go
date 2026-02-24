package skill

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Skill represents a loaded skill with YAML frontmatter metadata and Markdown body.
type Skill struct {
	// From YAML frontmatter
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags,omitempty"`

	// Parsed content
	Body     string // Markdown body (after frontmatter)
	FilePath string // Source file path
}

// Loader scans directories for SKILL.md files and loads them.
type Loader struct {
	skills []Skill
	mu     sync.RWMutex
}

// NewLoader creates a new skill loader.
func NewLoader() *Loader {
	return &Loader{}
}

// LoadFrom scans directories for SKILL.md files and loads all found skills.
// It accepts multiple directories (e.g. workspace/skills/ and ~/.goclaw/skills/).
func (l *Loader) LoadFrom(dirs ...string) error {
	var loaded []Skill

	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}

		err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // skip errors
			}
			if d.IsDir() {
				return nil
			}
			if strings.ToUpper(d.Name()) != "SKILL.MD" {
				return nil
			}

			skill, err := parseSkillFile(path)
			if err != nil {
				slog.Warn("failed to parse skill file", "path", path, "error", err)
				return nil
			}

			loaded = append(loaded, skill)
			slog.Info("loaded skill", "name", skill.Name, "path", path)
			return nil
		})
		if err != nil {
			slog.Warn("error scanning skills directory", "dir", dir, "error", err)
		}
	}

	l.mu.Lock()
	l.skills = loaded
	l.mu.Unlock()

	return nil
}

// Skills returns all loaded skills.
func (l *Loader) Skills() []Skill {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]Skill{}, l.skills...)
}

// MatchSkills returns skills relevant to the given user message.
// Relevance is determined by keyword matching against skill name,
// description, and tags.
func (l *Loader) MatchSkills(userMsg string, maxSkills int) []Skill {
	l.mu.RLock()
	skills := l.skills
	l.mu.RUnlock()

	if len(skills) == 0 {
		return nil
	}

	if maxSkills <= 0 {
		maxSkills = 3
	}

	msgLower := strings.ToLower(userMsg)
	msgWords := strings.Fields(msgLower)

	type scored struct {
		skill Skill
		score int
	}

	var results []scored

	for _, s := range skills {
		score := 0

		nameLower := strings.ToLower(s.Name)
		descLower := strings.ToLower(s.Description)

		// Check if any word from the message appears in skill metadata
		for _, word := range msgWords {
			if len(word) < 3 {
				continue // skip short words
			}
			if strings.Contains(nameLower, word) {
				score += 3
			}
			if strings.Contains(descLower, word) {
				score += 2
			}
			for _, tag := range s.Tags {
				if strings.Contains(strings.ToLower(tag), word) {
					score += 2
				}
			}
		}

		// Also check if skill name/tags appear in the message
		if strings.Contains(msgLower, nameLower) {
			score += 5
		}
		for _, tag := range s.Tags {
			if strings.Contains(msgLower, strings.ToLower(tag)) {
				score += 3
			}
		}

		if score > 0 {
			results = append(results, scored{skill: s, score: score})
		}
	}

	// Sort by score descending (simple insertion sort for small lists)
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].score > results[j-1].score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	// Return top N
	var matched []Skill
	for i := 0; i < len(results) && i < maxSkills; i++ {
		matched = append(matched, results[i].skill)
	}

	return matched
}

// parseSkillFile reads a SKILL.md file and parses its frontmatter and body.
func parseSkillFile(path string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}

	frontmatter, body := splitFrontmatter(string(data))

	var skill Skill
	if frontmatter != "" {
		if err := yaml.Unmarshal([]byte(frontmatter), &skill); err != nil {
			return Skill{}, err
		}
	}

	skill.Body = strings.TrimSpace(body)
	skill.FilePath = path

	// Default name from directory name if not set
	if skill.Name == "" {
		skill.Name = filepath.Base(filepath.Dir(path))
	}

	return skill, nil
}

// splitFrontmatter extracts YAML frontmatter (between --- delimiters) from Markdown.
func splitFrontmatter(content string) (frontmatter, body string) {
	content = strings.TrimSpace(content)

	if !strings.HasPrefix(content, "---") {
		return "", content
	}

	// Find the closing ---
	rest := content[3:]
	rest = strings.TrimLeft(rest, " \t")
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return "", content
	}

	frontmatter = rest[:endIdx]
	body = rest[endIdx+4:]

	// Trim the newline after closing ---
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	} else if len(body) > 1 && body[0] == '\r' && body[1] == '\n' {
		body = body[2:]
	}

	return frontmatter, body
}

// FormatForPrompt formats matched skills for injection into the system prompt.
func FormatForPrompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n## Available Skills\n\n")

	for _, s := range skills {
		b.WriteString("### ")
		b.WriteString(s.Name)
		b.WriteString("\n\n")
		if s.Body != "" {
			b.WriteString(s.Body)
			b.WriteString("\n\n")
		}
	}

	return b.String()
}
