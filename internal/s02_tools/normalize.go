package s02_tools

import "github.com/lupguo/go_learn_agent/pkg/llm"

// NormalizeMessages cleans up messages before an API call:
//  1. Find orphaned tool_use (no matching tool_result) -> append "(cancelled)" placeholder
//  2. Merge consecutive same-role messages
func NormalizeMessages(messages []llm.Message) []llm.Message {
	if len(messages) == 0 {
		return messages
	}

	// Pass 1: Collect existing tool_result IDs
	resultIDs := make(map[string]bool)
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.Type == llm.ContentTypeToolResult {
				resultIDs[block.ToolUseID] = true
			}
		}
	}

	// Pass 2: Find orphaned tool_use blocks and create placeholder results
	var placeholders []llm.Message
	for _, msg := range messages {
		if msg.Role != llm.RoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			if block.Type == llm.ContentTypeToolUse && !resultIDs[block.ID] {
				placeholders = append(placeholders, llm.Message{
					Role: llm.RoleUser,
					Content: []llm.ContentBlock{
						{Type: llm.ContentTypeToolResult, ToolUseID: block.ID, Content: "(cancelled)"},
					},
				})
			}
		}
	}

	// Combine original + placeholders
	all := make([]llm.Message, 0, len(messages)+len(placeholders))
	all = append(all, messages...)
	all = append(all, placeholders...)

	// Pass 3: Merge consecutive same-role messages
	merged := []llm.Message{all[0]}
	for _, msg := range all[1:] {
		last := &merged[len(merged)-1]
		if msg.Role == last.Role {
			last.Content = append(last.Content, msg.Content...)
		} else {
			merged = append(merged, msg)
		}
	}
	return merged
}
