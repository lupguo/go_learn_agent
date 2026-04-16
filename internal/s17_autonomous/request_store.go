package s17_autonomous

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type RequestRecord struct {
	RequestID  string  `json:"request_id"`
	Kind       string  `json:"kind"`
	From       string  `json:"from"`
	To         string  `json:"to"`
	Status     string  `json:"status"`
	CreatedAt  float64 `json:"created_at"`
	UpdatedAt  float64 `json:"updated_at"`
	ResolvedBy string  `json:"resolved_by,omitempty"`
	ResolvedAt float64 `json:"resolved_at,omitempty"`
	Plan       string  `json:"plan,omitempty"`
	ReviewedBy string  `json:"reviewed_by,omitempty"`
	Feedback   string  `json:"feedback,omitempty"`
}

type RequestStore struct {
	dir string
	mu  sync.Mutex
}

func NewRequestStore(dir string) (*RequestStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create requests dir: %w", err)
	}
	return &RequestStore{dir: dir}, nil
}

func (s *RequestStore) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func (s *RequestStore) Create(record *RequestRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := json.MarshalIndent(record, "", "  ")
	_ = os.WriteFile(s.path(record.RequestID), data, 0644)
}

func (s *RequestStore) Get(id string) *RequestRecord {
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return nil
	}
	var r RequestRecord
	if err := json.Unmarshal(data, &r); err != nil {
		return nil
	}
	return &r
}

func (s *RequestStore) Update(id string, fn func(*RequestRecord)) *RequestRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.Get(id)
	if r == nil {
		return nil
	}
	fn(r)
	r.UpdatedAt = float64(time.Now().Unix())
	data, _ := json.MarshalIndent(r, "", "  ")
	_ = os.WriteFile(s.path(id), data, 0644)
	return r
}
