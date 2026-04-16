package s16_protocols

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Valid message types for the inbox system.
var ValidMsgTypes = map[string]bool{
	"message":                true,
	"broadcast":              true,
	"shutdown_request":       true,
	"shutdown_response":      true,
	"plan_approval":          true,
	"plan_approval_response": true,
}

// InboxMessage is a single message stored in a JSONL inbox file.
type InboxMessage struct {
	Type      string  `json:"type"`
	From      string  `json:"from"`
	Content   string  `json:"content"`
	Timestamp float64 `json:"timestamp"`
	// Protocol fields (optional)
	RequestID string `json:"request_id,omitempty"`
	Approve   *bool  `json:"approve,omitempty"`
	Plan      string `json:"plan,omitempty"`
	Feedback  string `json:"feedback,omitempty"`
}

// MessageBus provides JSONL-file-based inboxes for inter-agent communication.
type MessageBus struct {
	dir string
	mu  sync.Mutex
}

// NewMessageBus creates a bus with the given inbox directory.
func NewMessageBus(inboxDir string) (*MessageBus, error) {
	if err := os.MkdirAll(inboxDir, 0755); err != nil {
		return nil, fmt.Errorf("create inbox dir: %w", err)
	}
	return &MessageBus{dir: inboxDir}, nil
}

// Send appends a message to a teammate's JSONL inbox.
func (b *MessageBus) Send(sender, to, content, msgType string, extra *InboxMessage) string {
	if msgType == "" {
		msgType = "message"
	}
	if !ValidMsgTypes[msgType] {
		return fmt.Sprintf("Error: Invalid type '%s'", msgType)
	}

	msg := InboxMessage{
		Type:      msgType,
		From:      sender,
		Content:   content,
		Timestamp: float64(time.Now().Unix()),
	}
	// Merge extra protocol fields
	if extra != nil {
		msg.RequestID = extra.RequestID
		msg.Approve = extra.Approve
		msg.Plan = extra.Plan
		msg.Feedback = extra.Feedback
	}
	data, _ := json.Marshal(msg)

	b.mu.Lock()
	defer b.mu.Unlock()

	path := filepath.Join(b.dir, to+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	defer f.Close()
	f.Write(append(data, '\n'))

	return fmt.Sprintf("Sent %s to %s", msgType, to)
}

// ReadInbox drains all messages from a teammate's inbox (read + clear).
func (b *MessageBus) ReadInbox(name string) []InboxMessage {
	b.mu.Lock()
	defer b.mu.Unlock()

	path := filepath.Join(b.dir, name+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var messages []InboxMessage
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var msg InboxMessage
		if err := json.Unmarshal([]byte(line), &msg); err == nil {
			messages = append(messages, msg)
		}
	}

	_ = os.WriteFile(path, nil, 0644)
	return messages
}

// Broadcast sends a message to all teammates except the sender.
func (b *MessageBus) Broadcast(sender, content string, teammates []string) string {
	count := 0
	for _, name := range teammates {
		if name != sender {
			b.Send(sender, name, content, "broadcast", nil)
			count++
		}
	}
	return fmt.Sprintf("Broadcast to %d teammates", count)
}
