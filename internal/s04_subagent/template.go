// Package s04_subagent demonstrates context isolation via subagents.
//
// Key insight: a subagent gets fresh messages=[] so the parent context stays clean.
// The child shares the filesystem but not conversation history, and only returns
// a text summary to the parent.
package s04_subagent

import (
	"os"
	"regexp"
	"strings"
)

// AgentTemplate parses agent definitions from markdown frontmatter.
// Real Claude Code loads these from .claude/agents/*.md.
type AgentTemplate struct {
	Name         string
	Config       map[string]string
	SystemPrompt string
}

var frontmatterRe = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)`)

// ParseAgentTemplate reads a markdown file with YAML-like frontmatter.
func ParseAgentTemplate(path string) (*AgentTemplate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	text := string(data)
	tmpl := &AgentTemplate{
		Name:   baseName(path),
		Config: make(map[string]string),
	}

	matches := frontmatterRe.FindStringSubmatch(text)
	if matches == nil {
		tmpl.SystemPrompt = strings.TrimSpace(text)
		return tmpl, nil
	}

	for _, line := range strings.Split(matches[1], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		tmpl.Config[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	tmpl.SystemPrompt = strings.TrimSpace(matches[2])
	if name, ok := tmpl.Config["name"]; ok {
		tmpl.Name = name
	}
	return tmpl, nil
}

// baseName returns the filename without extension from a path.
func baseName(path string) string {
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	return strings.TrimSuffix(name, ".md")
}
