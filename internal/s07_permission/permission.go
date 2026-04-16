// Package s07_permission implements a permission pipeline for tool call safety.
//
// Pipeline: deny rules → bash validator → mode check → allow rules → ask user
//
// Key insight: "Safety is a pipeline, not a boolean."
package s07_permission

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Mode determines the permission behavior for the agent session.
type Mode string

const (
	ModeDefault Mode = "default" // ask user for everything unmatched
	ModePlan    Mode = "plan"    // deny all write operations
	ModeAuto    Mode = "auto"    // auto-approve reads, ask for writes
)

// Decision is the result of a permission check.
type Decision struct {
	Behavior string // "allow", "deny", "ask"
	Reason   string
}

// Rule is a permission rule checked in order — first match wins.
type Rule struct {
	Tool     string // tool name or "*"
	Path     string // glob pattern or "*"
	Content  string // glob pattern for bash command
	Behavior string // "allow", "deny", "ask"
}

func (r Rule) String() string {
	parts := []string{fmt.Sprintf("tool=%s", r.Tool)}
	if r.Path != "" {
		parts = append(parts, fmt.Sprintf("path=%s", r.Path))
	}
	if r.Content != "" {
		parts = append(parts, fmt.Sprintf("content=%s", r.Content))
	}
	parts = append(parts, fmt.Sprintf("behavior=%s", r.Behavior))
	return "{" + strings.Join(parts, ", ") + "}"
}

// Write tools that modify state.
var writeTools = map[string]bool{
	"write_file": true,
	"edit_file":  true,
	"bash":       true,
}

// DefaultRules provides baseline permission rules.
var DefaultRules = []Rule{
	{Tool: "bash", Content: "rm -rf /*", Behavior: "deny"},
	{Tool: "bash", Content: "sudo *", Behavior: "deny"},
	{Tool: "read_file", Path: "*", Behavior: "allow"},
}

// --- Bash Security Validator ---

// BashValidator checks bash commands for dangerous patterns.
type BashValidator struct {
	validators []struct {
		name    string
		pattern *regexp.Regexp
	}
}

// NewBashValidator creates a validator with standard security patterns.
func NewBashValidator() *BashValidator {
	patterns := []struct {
		name    string
		pattern string
	}{
		{"shell_metachar", `[;&|` + "`" + `$]`},
		{"sudo", `\bsudo\b`},
		{"rm_rf", `\brm\s+(-[a-zA-Z]*)?r`},
		{"cmd_substitution", `\$\(`},
		{"ifs_injection", `\bIFS\s*=`},
	}

	bv := &BashValidator{}
	for _, p := range patterns {
		bv.validators = append(bv.validators, struct {
			name    string
			pattern *regexp.Regexp
		}{
			name:    p.name,
			pattern: regexp.MustCompile(p.pattern),
		})
	}
	return bv
}

// Validate returns a list of (name, pattern) failures. Empty means safe.
func (bv *BashValidator) Validate(command string) []string {
	var failures []string
	for _, v := range bv.validators {
		if v.pattern.MatchString(command) {
			failures = append(failures, fmt.Sprintf("%s (pattern: %s)", v.name, v.pattern.String()))
		}
	}
	return failures
}

// IsSafe returns true if no validators triggered.
func (bv *BashValidator) IsSafe(command string) bool {
	return len(bv.Validate(command)) == 0
}

// severePatterns are patterns that get immediate deny (no user override).
var severePatterns = map[string]bool{
	"sudo":  true,
	"rm_rf": true,
}

// --- Permission Manager ---

// PermissionManager manages permission decisions via a pipeline pattern.
type PermissionManager struct {
	Mode                  Mode
	Rules                 []Rule
	bashValidator         *BashValidator
	ConsecutiveDenials    int
	MaxConsecutiveDenials int
}

// NewPermissionManager creates a PermissionManager with the given mode.
func NewPermissionManager(mode Mode) *PermissionManager {
	rules := make([]Rule, len(DefaultRules))
	copy(rules, DefaultRules)
	return &PermissionManager{
		Mode:                  mode,
		Rules:                 rules,
		bashValidator:         NewBashValidator(),
		MaxConsecutiveDenials: 3,
	}
}

// Check runs the permission pipeline and returns a decision.
// Pipeline: bash validator → deny rules → mode check → allow rules → ask user
func (pm *PermissionManager) Check(toolName string, toolInput map[string]any) Decision {
	// Step 0: Bash security validation
	if toolName == "bash" {
		command, _ := toolInput["command"].(string)
		failures := pm.bashValidator.Validate(command)
		if len(failures) > 0 {
			// Check for severe patterns → immediate deny
			for _, f := range failures {
				for severe := range severePatterns {
					if strings.HasPrefix(f, severe) {
						return Decision{
							Behavior: "deny",
							Reason:   fmt.Sprintf("Bash validator: %s", strings.Join(failures, ", ")),
						}
					}
				}
			}
			// Non-severe → escalate to ask
			return Decision{
				Behavior: "ask",
				Reason:   fmt.Sprintf("Bash validator flagged: %s", strings.Join(failures, ", ")),
			}
		}
	}

	// Step 1: Deny rules (checked first, bypass-immune)
	for _, rule := range pm.Rules {
		if rule.Behavior != "deny" {
			continue
		}
		if matchesRule(rule, toolName, toolInput) {
			return Decision{
				Behavior: "deny",
				Reason:   fmt.Sprintf("Blocked by deny rule: %s", rule),
			}
		}
	}

	// Step 2: Mode-based decisions
	switch pm.Mode {
	case ModePlan:
		if writeTools[toolName] {
			return Decision{Behavior: "deny", Reason: "Plan mode: write operations are blocked"}
		}
		return Decision{Behavior: "allow", Reason: "Plan mode: read-only allowed"}

	case ModeAuto:
		if toolName == "read_file" {
			return Decision{Behavior: "allow", Reason: "Auto mode: read-only tool auto-approved"}
		}
		// fall through to allow rules then ask
	}

	// Step 3: Allow rules
	for _, rule := range pm.Rules {
		if rule.Behavior != "allow" {
			continue
		}
		if matchesRule(rule, toolName, toolInput) {
			pm.ConsecutiveDenials = 0
			return Decision{
				Behavior: "allow",
				Reason:   fmt.Sprintf("Matched allow rule: %s", rule),
			}
		}
	}

	// Step 4: Ask user
	return Decision{
		Behavior: "ask",
		Reason:   fmt.Sprintf("No rule matched for %s, asking user", toolName),
	}
}

// RecordApproval resets the denial counter.
func (pm *PermissionManager) RecordApproval() {
	pm.ConsecutiveDenials = 0
}

// RecordDenial increments the denial counter and returns a warning if threshold reached.
func (pm *PermissionManager) RecordDenial() string {
	pm.ConsecutiveDenials++
	if pm.ConsecutiveDenials >= pm.MaxConsecutiveDenials {
		return fmt.Sprintf("[%d consecutive denials — consider switching to plan mode]", pm.ConsecutiveDenials)
	}
	return ""
}

// AddAllowRule adds a permanent allow rule for a tool.
func (pm *PermissionManager) AddAllowRule(toolName string) {
	pm.Rules = append(pm.Rules, Rule{Tool: toolName, Path: "*", Behavior: "allow"})
	pm.ConsecutiveDenials = 0
}

// matchesRule checks if a rule matches the tool call.
func matchesRule(rule Rule, toolName string, toolInput map[string]any) bool {
	// Tool name match
	if rule.Tool != "" && rule.Tool != "*" && rule.Tool != toolName {
		return false
	}
	// Path glob match
	if rule.Path != "" && rule.Path != "*" {
		path, _ := toolInput["path"].(string)
		if matched, _ := filepath.Match(rule.Path, path); !matched {
			return false
		}
	}
	// Content glob match (for bash commands)
	if rule.Content != "" {
		command, _ := toolInput["command"].(string)
		if matched, _ := filepath.Match(rule.Content, command); !matched {
			return false
		}
	}
	return true
}
