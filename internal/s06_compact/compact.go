// Package s06_compact implements context compression to keep the active context manageable.
//
// Three strategies:
// 1. Large tool output → persist to disk, replace with preview marker
// 2. Older tool results → micro-compact into short placeholders
// 3. Whole conversation too large → LLM summarizes, continue from summary
package s06_compact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lupguo/go_learn_agent/pkg/llm"
)

const (
	ContextLimit          = 50000
	KeepRecentToolResults = 3
	PersistThreshold      = 30000
	PreviewChars          = 2000
)

// CompactState tracks compression state across the session.
type CompactState struct {
	HasCompacted bool
	LastSummary  string
	RecentFiles  []string
}

// TrackRecentFile records a file path for potential re-opening after compact.
func (cs *CompactState) TrackRecentFile(path string) {
	// Remove if already present (move to end)
	for i, f := range cs.RecentFiles {
		if f == path {
			cs.RecentFiles = append(cs.RecentFiles[:i], cs.RecentFiles[i+1:]...)
			break
		}
	}
	cs.RecentFiles = append(cs.RecentFiles, path)
	if len(cs.RecentFiles) > 5 {
		cs.RecentFiles = cs.RecentFiles[len(cs.RecentFiles)-5:]
	}
}

// EstimateContextSize returns a rough character-count estimate of messages.
func EstimateContextSize(messages []llm.Message) int {
	data, _ := json.Marshal(messages)
	return len(data)
}

// PersistLargeOutput saves output to disk if it exceeds the threshold,
// returning a preview marker. Otherwise returns the original output.
func PersistLargeOutput(workDir, toolUseID, output string) string {
	if len(output) <= PersistThreshold {
		return output
	}

	dir := filepath.Join(workDir, ".task_outputs", "tool-results")
	os.MkdirAll(dir, 0o755)
	storedPath := filepath.Join(dir, toolUseID+".txt")

	if _, err := os.Stat(storedPath); os.IsNotExist(err) {
		os.WriteFile(storedPath, []byte(output), 0o644)
	}

	preview := output
	if len(preview) > PreviewChars {
		preview = preview[:PreviewChars]
	}

	relPath, _ := filepath.Rel(workDir, storedPath)
	return fmt.Sprintf("<persisted-output>\nFull output saved to: %s\nPreview:\n%s\n</persisted-output>", relPath, preview)
}

// MicroCompact replaces older tool results with short placeholders,
// keeping only the N most recent results intact.
func MicroCompact(messages []llm.Message) []llm.Message {
	// Collect indices of all tool_result blocks
	type resultRef struct {
		msgIdx   int
		blockIdx int
	}
	var refs []resultRef

	for mi, msg := range messages {
		if msg.Role != llm.RoleUser {
			continue
		}
		for bi, block := range msg.Content {
			if block.Type == llm.ContentTypeToolResult {
				refs = append(refs, resultRef{mi, bi})
			}
		}
	}

	if len(refs) <= KeepRecentToolResults {
		return messages
	}

	// Compact older results
	for _, ref := range refs[:len(refs)-KeepRecentToolResults] {
		block := &messages[ref.msgIdx].Content[ref.blockIdx]
		if len(block.Content) > 120 {
			block.Content = "[Earlier tool result compacted. Re-run the tool if you need full detail.]"
		}
	}

	return messages
}

// WriteTranscript saves the full conversation to a JSONL file before compacting.
func WriteTranscript(workDir string, messages []llm.Message) string {
	dir := filepath.Join(workDir, ".transcripts")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, fmt.Sprintf("transcript_%d.jsonl", time.Now().Unix()))

	f, err := os.Create(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, msg := range messages {
		enc.Encode(msg)
	}
	return path
}
