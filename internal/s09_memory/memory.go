// Package s09_memory implements cross-session persistent memory.
//
// Memory stores information that should survive the current conversation
// but is NOT derivable from the repo itself: user preferences, feedback,
// project decisions, external resource pointers.
//
// Storage layout:
//
//	.memory/
//	  MEMORY.md          <- compact index (≤200 lines)
//	  prefer_tabs.md     <- individual memory with frontmatter
//	  review_style.md
//
// Key insight: "Memory ≠ Context — memory is persistent, context is temporary."
package s09_memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// MemoryTypes are the valid categories for a memory entry.
var MemoryTypes = []string{"user", "feedback", "project", "reference"}

const maxIndexLines = 200

var (
	frontmatterRe = regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---\s*\n(.*)`)
	safeNameRe    = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
)

// MemoryEntry is one loaded memory.
type MemoryEntry struct {
	Name        string
	Description string
	Type        string
	Content     string
	File        string
}

// MemoryManager handles loading, saving, and indexing persistent memories.
type MemoryManager struct {
	memoryDir string
	Memories  map[string]*MemoryEntry
}

// NewMemoryManager creates a manager for the given directory.
func NewMemoryManager(memoryDir string) *MemoryManager {
	return &MemoryManager{
		memoryDir: memoryDir,
		Memories:  make(map[string]*MemoryEntry),
	}
}

// LoadAll scans .md files in memoryDir and populates in-memory state.
func (mm *MemoryManager) LoadAll() {
	mm.Memories = make(map[string]*MemoryEntry)

	entries, err := os.ReadDir(mm.memoryDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "MEMORY.md" {
			continue
		}
		path := filepath.Join(mm.memoryDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parsed := parseFrontmatter(string(data))
		if parsed == nil {
			continue
		}
		name := parsed["name"]
		if name == "" {
			name = strings.TrimSuffix(entry.Name(), ".md")
		}
		mm.Memories[name] = &MemoryEntry{
			Name:        name,
			Description: parsed["description"],
			Type:        orDefault(parsed["type"], "project"),
			Content:     parsed["content"],
			File:        entry.Name(),
		}
	}

	if len(mm.Memories) > 0 {
		fmt.Printf("[Memory loaded: %d memories from %s]\n", len(mm.Memories), mm.memoryDir)
	}
}

// MemoryPrompt builds a section for injection into the system prompt.
func (mm *MemoryManager) MemoryPrompt() string {
	if len(mm.Memories) == 0 {
		return ""
	}

	var sections []string
	sections = append(sections, "# Memories (persistent across sessions)", "")

	for _, memType := range MemoryTypes {
		var typed []*MemoryEntry
		for _, mem := range mm.Memories {
			if mem.Type == memType {
				typed = append(typed, mem)
			}
		}
		if len(typed) == 0 {
			continue
		}
		// Sort for stable output
		sort.Slice(typed, func(i, j int) bool { return typed[i].Name < typed[j].Name })

		sections = append(sections, fmt.Sprintf("## [%s]", memType))
		for _, mem := range typed {
			sections = append(sections, fmt.Sprintf("### %s: %s", mem.Name, mem.Description))
			if content := strings.TrimSpace(mem.Content); content != "" {
				sections = append(sections, content)
			}
			sections = append(sections, "")
		}
	}

	return strings.Join(sections, "\n")
}

// SaveMemory writes a memory file and rebuilds the index. Returns a status message.
func (mm *MemoryManager) SaveMemory(name, description, memType, content string) (string, error) {
	if !isValidType(memType) {
		return "", fmt.Errorf("type must be one of %v", MemoryTypes)
	}

	safeName := safeNameRe.ReplaceAllString(strings.ToLower(name), "_")
	if safeName == "" {
		return "", fmt.Errorf("invalid memory name")
	}

	os.MkdirAll(mm.memoryDir, 0o755)

	// Write memory file with frontmatter
	fileName := safeName + ".md"
	filePath := filepath.Join(mm.memoryDir, fileName)
	frontmatter := fmt.Sprintf("---\nname: %s\ndescription: %s\ntype: %s\n---\n%s\n", name, description, memType, content)
	if err := os.WriteFile(filePath, []byte(frontmatter), 0o644); err != nil {
		return "", fmt.Errorf("write failed: %w", err)
	}

	// Update in-memory store
	mm.Memories[name] = &MemoryEntry{
		Name:        name,
		Description: description,
		Type:        memType,
		Content:     content,
		File:        fileName,
	}

	// Rebuild index
	mm.rebuildIndex()

	relPath, _ := filepath.Rel(filepath.Dir(mm.memoryDir), filePath)
	return fmt.Sprintf("Saved memory '%s' [%s] to %s", name, memType, relPath), nil
}

func (mm *MemoryManager) rebuildIndex() {
	var lines []string
	lines = append(lines, "# Memory Index", "")
	count := 0
	for name, mem := range mm.Memories {
		lines = append(lines, fmt.Sprintf("- %s: %s [%s]", name, mem.Description, mem.Type))
		count++
		if count >= maxIndexLines {
			lines = append(lines, fmt.Sprintf("... (truncated at %d lines)", maxIndexLines))
			break
		}
	}
	indexPath := filepath.Join(mm.memoryDir, "MEMORY.md")
	os.WriteFile(indexPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func parseFrontmatter(text string) map[string]string {
	matches := frontmatterRe.FindStringSubmatch(text)
	if matches == nil {
		return nil
	}
	result := map[string]string{"content": strings.TrimSpace(matches[2])}
	for _, line := range strings.Split(matches[1], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return result
}

func isValidType(t string) bool {
	for _, valid := range MemoryTypes {
		if t == valid {
			return true
		}
	}
	return false
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
