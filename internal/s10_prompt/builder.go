// Package s10_prompt implements a section-based system prompt assembly pipeline.
//
// Pipeline sections:
//  1. Core instructions
//  2. Tool listing
//  3. Skill metadata (layer 1 catalog)
//  4. Memory content
//  5. CLAUDE.md chain (global → project → subdir)
//  6. Dynamic context (date, platform, cwd)
//
// A DYNAMIC_BOUNDARY marker separates stable (cacheable) sections from
// per-turn dynamic context.
//
// Key insight: "Prompt construction is a pipeline with boundaries, not one big string."
package s10_prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/lupguo/go_learn_agent/pkg/llm"
)

const DynamicBoundary = "=== DYNAMIC_BOUNDARY ==="

var fmRegex = regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---`)

// SystemPromptBuilder assembles the system prompt from independent sections.
type SystemPromptBuilder struct {
	workDir   string
	tools     []llm.ToolDef
	skillsDir string
	memoryDir string
	model     string
}

// NewBuilder creates a SystemPromptBuilder.
func NewBuilder(workDir string, tools []llm.ToolDef, model string) *SystemPromptBuilder {
	return &SystemPromptBuilder{
		workDir:   workDir,
		tools:     tools,
		skillsDir: filepath.Join(workDir, "skills"),
		memoryDir: filepath.Join(workDir, ".memory"),
		model:     model,
	}
}

// Build assembles the full system prompt from all sections.
func (b *SystemPromptBuilder) Build() string {
	var sections []string

	if s := b.buildCore(); s != "" {
		sections = append(sections, s)
	}
	if s := b.buildToolListing(); s != "" {
		sections = append(sections, s)
	}
	if s := b.buildSkillListing(); s != "" {
		sections = append(sections, s)
	}
	if s := b.buildMemorySection(); s != "" {
		sections = append(sections, s)
	}
	if s := b.buildClaudeMD(); s != "" {
		sections = append(sections, s)
	}

	// Static/dynamic boundary
	sections = append(sections, DynamicBoundary)

	if s := b.buildDynamicContext(); s != "" {
		sections = append(sections, s)
	}

	return strings.Join(sections, "\n\n")
}

// Section 1: Core instructions
func (b *SystemPromptBuilder) buildCore() string {
	return fmt.Sprintf(
		"You are a coding agent operating in %s.\n"+
			"Use the provided tools to explore, read, write, and edit files.\n"+
			"Always verify before assuming. Prefer reading files over guessing.",
		b.workDir,
	)
}

// Section 2: Tool listings
func (b *SystemPromptBuilder) buildToolListing() string {
	if len(b.tools) == 0 {
		return ""
	}
	lines := []string{"# Available tools"}
	for _, t := range b.tools {
		// Extract parameter names from schema
		params := extractParamNames(t.InputSchema)
		lines = append(lines, fmt.Sprintf("- %s(%s): %s", t.Name, strings.Join(params, ", "), t.Description))
	}
	return strings.Join(lines, "\n")
}

// Section 3: Skill metadata (layer 1 catalog from s05)
func (b *SystemPromptBuilder) buildSkillListing() string {
	entries, err := os.ReadDir(b.skillsDir)
	if err != nil {
		return ""
	}

	var skills []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(b.skillsDir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}
		meta := parseFrontmatterMeta(string(data))
		name := meta["name"]
		if name == "" {
			name = entry.Name()
		}
		desc := meta["description"]
		skills = append(skills, fmt.Sprintf("- %s: %s", name, desc))
	}

	if len(skills) == 0 {
		return ""
	}
	return "# Available skills\n" + strings.Join(skills, "\n")
}

// Section 4: Memory content
func (b *SystemPromptBuilder) buildMemorySection() string {
	entries, err := os.ReadDir(b.memoryDir)
	if err != nil {
		return ""
	}

	var memories []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "MEMORY.md" || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(b.memoryDir, entry.Name()))
		if err != nil {
			continue
		}
		text := string(data)
		matches := regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---\s*\n(.*)`).FindStringSubmatch(text)
		if matches == nil {
			continue
		}
		meta := parseKeyValues(matches[1])
		body := strings.TrimSpace(matches[2])
		name := meta["name"]
		if name == "" {
			name = strings.TrimSuffix(entry.Name(), ".md")
		}
		memType := meta["type"]
		if memType == "" {
			memType = "project"
		}
		desc := meta["description"]
		memories = append(memories, fmt.Sprintf("[%s] %s: %s\n%s", memType, name, desc, body))
	}

	if len(memories) == 0 {
		return ""
	}
	return "# Memories (persistent)\n\n" + strings.Join(memories, "\n\n")
}

// Section 5: CLAUDE.md chain (global → project → subdir)
func (b *SystemPromptBuilder) buildClaudeMD() string {
	type source struct {
		label   string
		content string
	}
	var sources []source

	// User-global: ~/.claude/CLAUDE.md
	home, _ := os.UserHomeDir()
	if home != "" {
		userClaude := filepath.Join(home, ".claude", "CLAUDE.md")
		if data, err := os.ReadFile(userClaude); err == nil {
			sources = append(sources, source{"user global (~/.claude/CLAUDE.md)", string(data)})
		}
	}

	// Project root
	projectClaude := filepath.Join(b.workDir, "CLAUDE.md")
	if data, err := os.ReadFile(projectClaude); err == nil {
		sources = append(sources, source{"project root (CLAUDE.md)", string(data)})
	}

	// Subdirectory (if cwd differs from workDir)
	cwd, _ := os.Getwd()
	if cwd != b.workDir {
		subdirClaude := filepath.Join(cwd, "CLAUDE.md")
		if data, err := os.ReadFile(subdirClaude); err == nil {
			dir := filepath.Base(cwd)
			sources = append(sources, source{fmt.Sprintf("subdir (%s/CLAUDE.md)", dir), string(data)})
		}
	}

	if len(sources) == 0 {
		return ""
	}

	parts := []string{"# CLAUDE.md instructions"}
	for _, s := range sources {
		parts = append(parts, fmt.Sprintf("## From %s", s.label))
		parts = append(parts, strings.TrimSpace(s.content))
	}
	return strings.Join(parts, "\n\n")
}

// Section 6: Dynamic context
func (b *SystemPromptBuilder) buildDynamicContext() string {
	lines := []string{
		fmt.Sprintf("Current date: %s", time.Now().Format("2006-01-02")),
		fmt.Sprintf("Working directory: %s", b.workDir),
		fmt.Sprintf("Model: %s", b.model),
		fmt.Sprintf("Platform: %s", runtime.GOOS),
	}
	return "# Dynamic context\n" + strings.Join(lines, "\n")
}

// BuildSystemReminder creates a user-role system reminder for per-turn dynamic content.
func BuildSystemReminder(extra string) *llm.Message {
	if extra == "" {
		return nil
	}
	msg := llm.NewTextMessage(llm.RoleUser, "<system-reminder>\n"+extra+"\n</system-reminder>")
	return &msg
}

// --- helpers ---

func parseFrontmatterMeta(text string) map[string]string {
	matches := fmRegex.FindStringSubmatch(text)
	if matches == nil {
		return nil
	}
	return parseKeyValues(matches[1])
}

func parseKeyValues(block string) map[string]string {
	meta := make(map[string]string)
	for _, line := range strings.Split(block, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		meta[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return meta
}

func extractParamNames(schema any) []string {
	m, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	props, ok := m["properties"].(map[string]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(props))
	for k := range props {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
