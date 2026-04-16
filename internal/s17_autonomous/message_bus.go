package s17_autonomous

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var ValidMsgTypes = map[string]bool{
	"message": true, "broadcast": true,
	"shutdown_request": true, "shutdown_response": true,
	"plan_approval": true, "plan_approval_response": true,
}

type InboxMessage struct {
	Type      string  `json:"type"`
	From      string  `json:"from"`
	Content   string  `json:"content"`
	Timestamp float64 `json:"timestamp"`
	RequestID string  `json:"request_id,omitempty"`
	Approve   *bool   `json:"approve,omitempty"`
	Plan      string  `json:"plan,omitempty"`
	Feedback  string  `json:"feedback,omitempty"`
}

type MessageBus struct {
	dir string
	mu  sync.Mutex
}

func NewMessageBus(inboxDir string) (*MessageBus, error) {
	if err := os.MkdirAll(inboxDir, 0755); err != nil {
		return nil, fmt.Errorf("create inbox dir: %w", err)
	}
	return &MessageBus{dir: inboxDir}, nil
}

func (b *MessageBus) Send(sender, to, content, msgType string, extra *InboxMessage) string {
	if msgType == "" {
		msgType = "message"
	}
	if !ValidMsgTypes[msgType] {
		return fmt.Sprintf("Error: Invalid type '%s'", msgType)
	}
	msg := InboxMessage{
		Type: msgType, From: sender, Content: content,
		Timestamp: float64(time.Now().Unix()),
	}
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
