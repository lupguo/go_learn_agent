// Package s05_skills implements a two-layer skill loading model.
//
// Layer 1: cheap catalog (name + description) injected into system prompt.
// Layer 2: full skill body loaded on-demand when the model requests it.
// This keeps the prompt small while giving the model access to reusable guidance.
package s05_skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SkillManifest is the lightweight catalog entry for a skill.
type SkillManifest struct {
	Name        string
	Description string
	Path        string
}

// SkillDocument holds both the manifest and the full body text.
type SkillDocument struct {
	Manifest SkillManifest
	Body     string
}

var fmRegex = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)`)

// SkillRegistry scans a directory for SKILL.md files and provides two-layer access.
type SkillRegistry struct {
	documents map[string]*SkillDocument
}

// NewSkillRegistry scans skillsDir for SKILL.md files and builds the catalog.
func NewSkillRegistry(skillsDir string) *SkillRegistry {
	sr := &SkillRegistry{
		documents: make(map[string]*SkillDocument),
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return sr
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}
		meta, body := parseFrontmatter(string(data))
		name := meta["name"]
		if name == "" {
			name = entry.Name()
		}
		desc := meta["description"]
		if desc == "" {
			desc = "No description"
		}
		sr.documents[name] = &SkillDocument{
			Manifest: SkillManifest{
				Name:        name,
				Description: desc,
				Path:        skillFile,
			},
			Body: strings.TrimSpace(body),
		}
	}

	return sr
}

func parseFrontmatter(text string) (meta map[string]string, body string) {
	meta = make(map[string]string)
	matches := fmRegex.FindStringSubmatch(text)
	if matches == nil {
		return meta, text
	}
	for _, line := range strings.Split(matches[1], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		meta[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return meta, matches[2]
}

// DescribeAvailable returns a compact catalog string for system prompt injection.
func (sr *SkillRegistry) DescribeAvailable() string {
	if len(sr.documents) == 0 {
		return "(no skills available)"
	}
	names := make([]string, 0, len(sr.documents))
	for name := range sr.documents {
		names = append(names, name)
	}
	sort.Strings(names)

	var lines []string
	for _, name := range names {
		m := sr.documents[name].Manifest
		lines = append(lines, fmt.Sprintf("- %s: %s", m.Name, m.Description))
	}
	return strings.Join(lines, "\n")
}

// LoadFullText returns the full skill body wrapped in XML tags.
func (sr *SkillRegistry) LoadFullText(name string) string {
	doc, ok := sr.documents[name]
	if !ok {
		known := make([]string, 0, len(sr.documents))
		for n := range sr.documents {
			known = append(known, n)
		}
		sort.Strings(known)
		return fmt.Sprintf("Error: Unknown skill %q. Available: %s", name, strings.Join(known, ", "))
	}
	return fmt.Sprintf("<skill name=%q>\n%s\n</skill>", doc.Manifest.Name, doc.Body)
}
