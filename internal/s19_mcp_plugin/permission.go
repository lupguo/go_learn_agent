package s19_mcp_plugin

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Permission modes.
const (
	ModeDefault = "default"
	ModeAuto    = "auto"
)

// Risk levels.
const (
	RiskRead  = "read"
	RiskWrite = "write"
	RiskHigh  = "high"
)

// Behavior for permission decisions.
const (
	BehaviorAllow = "allow"
	BehaviorAsk   = "ask"
	BehaviorDeny  = "deny"
)

// CapabilityIntent describes a normalized tool call.
type CapabilityIntent struct {
	Source string `json:"source"` // "native" or "mcp"
	Server string `json:"server"` // MCP server name or ""
	Tool   string `json:"tool"`   // actual tool name
	Risk   string `json:"risk"`   // read, write, high
}

// PermissionDecision is the result of a permission check.
type PermissionDecision struct {
	Behavior string           `json:"behavior"` // allow, ask, deny
	Reason   string           `json:"reason"`
	Intent   CapabilityIntent `json:"intent"`
}

// CapabilityPermissionGate provides shared permission checks for native and MCP tools.
type CapabilityPermissionGate struct {
	Mode string
}

func NewPermissionGate(mode string) *CapabilityPermissionGate {
	if mode != ModeDefault && mode != ModeAuto {
		mode = ModeDefault
	}
	return &CapabilityPermissionGate{Mode: mode}
}

var readPrefixes = []string{"read", "list", "get", "show", "search", "query", "inspect"}
var highRiskPrefixes = []string{"delete", "remove", "drop", "shutdown"}

// Normalize classifies a tool call into a CapabilityIntent.
func (g *CapabilityPermissionGate) Normalize(toolName string, toolInput map[string]any) CapabilityIntent {
	source := "native"
	server := ""
	actualTool := toolName

	if strings.HasPrefix(toolName, "mcp__") {
		parts := strings.SplitN(toolName, "__", 3)
		if len(parts) == 3 {
			source = "mcp"
			server = parts[1]
			actualTool = parts[2]
		}
	}

	lowered := strings.ToLower(actualTool)
	risk := RiskWrite

	if actualTool == "read_file" || hasAnyPrefix(lowered, readPrefixes) {
		risk = RiskRead
	} else if actualTool == "bash" {
		command, _ := toolInput["command"].(string)
		if containsAny(command, []string{"rm -rf", "sudo", "shutdown", "reboot"}) {
			risk = RiskHigh
		}
	} else if hasAnyPrefix(lowered, highRiskPrefixes) {
		risk = RiskHigh
	}

	return CapabilityIntent{Source: source, Server: server, Tool: actualTool, Risk: risk}
}

// Check returns a permission decision for a tool call.
func (g *CapabilityPermissionGate) Check(toolName string, toolInput map[string]any) PermissionDecision {
	intent := g.Normalize(toolName, toolInput)

	if intent.Risk == RiskRead {
		return PermissionDecision{Behavior: BehaviorAllow, Reason: "Read capability", Intent: intent}
	}
	if g.Mode == ModeAuto && intent.Risk != RiskHigh {
		return PermissionDecision{Behavior: BehaviorAllow, Reason: "Auto mode for non-high-risk capability", Intent: intent}
	}
	if intent.Risk == RiskHigh {
		return PermissionDecision{Behavior: BehaviorAsk, Reason: "High-risk capability requires confirmation", Intent: intent}
	}
	return PermissionDecision{Behavior: BehaviorAsk, Reason: "State-changing capability requires confirmation", Intent: intent}
}

// FormatAskPrompt returns a human-readable prompt for permission confirmation.
func (g *CapabilityPermissionGate) FormatAskPrompt(intent CapabilityIntent, toolInput map[string]any) string {
	preview, _ := json.Marshal(toolInput)
	if len(preview) > 200 {
		preview = append(preview[:200], []byte("...")...)
	}
	var source string
	if intent.Server != "" {
		source = fmt.Sprintf("%s:%s/%s", intent.Source, intent.Server, intent.Tool)
	} else {
		source = fmt.Sprintf("%s:%s", intent.Source, intent.Tool)
	}
	return fmt.Sprintf("[Permission] %s risk=%s: %s", source, intent.Risk, string(preview))
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
