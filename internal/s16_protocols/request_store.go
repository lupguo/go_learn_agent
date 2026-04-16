package s16_protocols

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RequestRecord is a durable protocol request stored as JSON on disk.
type RequestRecord struct {
	RequestID  string  `json:"request_id"`
	Kind       string  `json:"kind"` // "shutdown" or "plan_approval"
	From       string  `json:"from"`
	To         string  `json:"to"`
	Status     string  `json:"status"` // pending, approved, rejected
	CreatedAt  float64 `json:"created_at"`
	UpdatedAt  float64 `json:"updated_at"`
	// Shutdown-specific
	ResolvedBy string `json:"resolved_by,omitempty"`
	ResolvedAt float64 `json:"resolved_at,omitempty"`
	// Plan-specific
	Plan       string `json:"plan,omitempty"`
	ReviewedBy string `json:"reviewed_by,omitempty"`
	Feedback   string `json:"feedback,omitempty"`
}

// RequestStore manages durable request records in .team/requests/.
type RequestStore struct {
	dir string
	mu  sync.Mutex
}

// NewRequestStore creates a store backed by the given directory.
func NewRequestStore(dir string) (*RequestStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create requests dir: %w", err)
	}
	return &RequestStore{dir: dir}, nil
}

func (s *RequestStore) path(requestID string) string {
	return filepath.Join(s.dir, requestID+".json")
}

// Create persists a new request record.
func (s *RequestStore) Create(record *RequestRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, _ := json.MarshalIndent(record, "", "  ")
	_ = os.WriteFile(s.path(record.RequestID), data, 0644)
}

// Get loads a request record by ID.
func (s *RequestStore) Get(requestID string) *RequestRecord {
	data, err := os.ReadFile(s.path(requestID))
	if err != nil {
		return nil
	}
	var record RequestRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil
	}
	return &record
}

// Update modifies fields of an existing request record.
func (s *RequestStore) Update(requestID string, updateFn func(*RequestRecord)) *RequestRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := s.Get(requestID)
	if record == nil {
		return nil
	}
	updateFn(record)
	record.UpdatedAt = float64(time.Now().Unix())
	data, _ := json.MarshalIndent(record, "", "  ")
	_ = os.WriteFile(s.path(requestID), data, 0644)
	return record
}
