package s14_cron

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	AutoExpiryDays   = 7
	JitterOffsetMax  = 4
	CheckInterval    = 1 * time.Second
)

// jitterMinutes are exact minute values that recurring tasks should avoid.
var jitterMinutes = map[int]bool{0: true, 30: true}

// CronTask is a single scheduled work item.
type CronTask struct {
	ID           string  `json:"id"`
	Cron         string  `json:"cron"`
	Prompt       string  `json:"prompt"`
	Recurring    bool    `json:"recurring"`
	Durable      bool    `json:"durable"`
	CreatedAt    float64 `json:"createdAt"`
	LastFired    float64 `json:"last_fired,omitempty"`
	JitterOffset int     `json:"jitter_offset,omitempty"`
}

// CronScheduler manages scheduled tasks with a background checking goroutine.
type CronScheduler struct {
	tasks          []CronTask
	notifications  []string
	mu             sync.Mutex
	stopCh         chan struct{}
	durablePath    string
	lastCheckMin   int
	nextSeq        int
}

// NewCronScheduler creates a new scheduler. durablePath is the JSON file for persistent tasks.
func NewCronScheduler(durablePath string) *CronScheduler {
	return &CronScheduler{
		stopCh:       make(chan struct{}),
		durablePath:  durablePath,
		lastCheckMin: -1,
	}
}

// Start loads durable tasks and launches the background check goroutine.
func (s *CronScheduler) Start() {
	s.loadDurable()
	count := len(s.tasks)
	if count > 0 {
		fmt.Printf("[Cron] Loaded %d scheduled tasks\n", count)
	}
	go s.checkLoop()
}

// Stop signals the background goroutine to exit.
func (s *CronScheduler) Stop() {
	close(s.stopCh)
}

// Create adds a new scheduled task and returns a confirmation string.
func (s *CronScheduler) Create(cronExpr, prompt string, recurring, durable bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextSeq++
	taskID := fmt.Sprintf("cron_%04d", s.nextSeq)

	task := CronTask{
		ID:        taskID,
		Cron:      cronExpr,
		Prompt:    prompt,
		Recurring: recurring,
		Durable:   durable,
		CreatedAt: float64(time.Now().Unix()),
	}

	if recurring {
		task.JitterOffset = computeJitter(cronExpr)
	}

	s.tasks = append(s.tasks, task)
	if durable {
		s.saveDurable()
	}

	mode := "recurring"
	if !recurring {
		mode = "one-shot"
	}
	store := "session-only"
	if durable {
		store = "durable"
	}
	return fmt.Sprintf("Created task %s (%s, %s): cron=%s", taskID, mode, store, cronExpr)
}

// Delete removes a scheduled task by ID.
func (s *CronScheduler) Delete(taskID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	before := len(s.tasks)
	filtered := make([]CronTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		if t.ID != taskID {
			filtered = append(filtered, t)
		}
	}
	s.tasks = filtered

	if len(s.tasks) < before {
		s.saveDurable()
		return fmt.Sprintf("Deleted task %s", taskID)
	}
	return fmt.Sprintf("Task %s not found", taskID)
}

// ListTasks returns a formatted listing of all scheduled tasks.
func (s *CronScheduler) ListTasks() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.tasks) == 0 {
		return "No scheduled tasks."
	}

	var lines []string
	for _, t := range s.tasks {
		mode := "recurring"
		if !t.Recurring {
			mode = "one-shot"
		}
		store := "session"
		if t.Durable {
			store = "durable"
		}
		ageHours := (float64(time.Now().Unix()) - t.CreatedAt) / 3600
		promptPreview := t.Prompt
		if len(promptPreview) > 60 {
			promptPreview = promptPreview[:60]
		}
		lines = append(lines, fmt.Sprintf("  %s  %s  [%s/%s] (%.1fh old): %s",
			t.ID, t.Cron, mode, store, ageHours, promptPreview))
	}
	return strings.Join(lines, "\n")
}

// DrainNotifications returns and clears all pending cron notifications.
func (s *CronScheduler) DrainNotifications() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	notifs := make([]string, len(s.notifications))
	copy(notifs, s.notifications)
	s.notifications = s.notifications[:0]
	return notifs
}

// checkLoop runs in a goroutine, checking every second for due tasks.
func (s *CronScheduler) checkLoop() {
	ticker := time.NewTicker(CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			currentMinute := now.Hour()*60 + now.Minute()

			s.mu.Lock()
			if currentMinute != s.lastCheckMin {
				s.lastCheckMin = currentMinute
				s.checkTasks(now)
			}
			s.mu.Unlock()
		}
	}
}

// checkTasks fires matching tasks, handles expiry and one-shot cleanup. Must be called with mu held.
func (s *CronScheduler) checkTasks(now time.Time) {
	var expiredIDs, oneshotIDs []string

	for i := range s.tasks {
		t := &s.tasks[i]

		// Auto-expiry: recurring tasks older than 7 days
		ageDays := (float64(now.Unix()) - t.CreatedAt) / 86400
		if t.Recurring && ageDays > AutoExpiryDays {
			expiredIDs = append(expiredIDs, t.ID)
			continue
		}

		// Apply jitter offset
		checkTime := now
		if t.JitterOffset > 0 {
			checkTime = now.Add(-time.Duration(t.JitterOffset) * time.Minute)
		}

		if CronMatches(t.Cron, checkTime) {
			notification := fmt.Sprintf("[Scheduled task %s]: %s", t.ID, t.Prompt)
			s.notifications = append(s.notifications, notification)
			t.LastFired = float64(now.Unix())
			fmt.Printf("[Cron] Fired: %s\n", t.ID)

			if !t.Recurring {
				oneshotIDs = append(oneshotIDs, t.ID)
			}
		}
	}

	// Clean up
	if len(expiredIDs) > 0 || len(oneshotIDs) > 0 {
		removeSet := make(map[string]bool)
		for _, id := range expiredIDs {
			removeSet[id] = true
			fmt.Printf("[Cron] Auto-expired: %s (older than %d days)\n", id, AutoExpiryDays)
		}
		for _, id := range oneshotIDs {
			removeSet[id] = true
			fmt.Printf("[Cron] One-shot completed and removed: %s\n", id)
		}

		filtered := make([]CronTask, 0, len(s.tasks))
		for _, t := range s.tasks {
			if !removeSet[t.ID] {
				filtered = append(filtered, t)
			}
		}
		s.tasks = filtered
		s.saveDurable()
	}
}

func (s *CronScheduler) loadDurable() {
	if s.durablePath == "" {
		return
	}
	data, err := os.ReadFile(s.durablePath)
	if err != nil {
		return
	}
	var tasks []CronTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		fmt.Printf("[Cron] Error loading tasks: %v\n", err)
		return
	}
	// Only load durable tasks
	for _, t := range tasks {
		if t.Durable {
			s.tasks = append(s.tasks, t)
		}
	}
}

func (s *CronScheduler) saveDurable() {
	if s.durablePath == "" {
		return
	}
	var durable []CronTask
	for _, t := range s.tasks {
		if t.Durable {
			durable = append(durable, t)
		}
	}
	dir := filepath.Dir(s.durablePath)
	_ = os.MkdirAll(dir, 0755)
	data, _ := json.MarshalIndent(durable, "", "  ")
	_ = os.WriteFile(s.durablePath, append(data, '\n'), 0644)
}

// --- Cron expression matching ---

// CronMatches checks if a 5-field cron expression matches the given time.
// Fields: minute hour day-of-month month day-of-week
func CronMatches(expr string, t time.Time) bool {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false
	}

	// Go Weekday: Sunday=0; cron: Sunday=0 — already aligned
	values := []int{t.Minute(), t.Hour(), t.Day(), int(t.Month()), int(t.Weekday())}
	ranges := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}

	for i, field := range fields {
		if !fieldMatches(field, values[i], ranges[i][0], ranges[i][1]) {
			return false
		}
	}
	return true
}

// fieldMatches checks a single cron field against a value.
// Supports: * (any), */N (step), N (exact), N-M (range), N,M (list), combinations.
func fieldMatches(field string, value, lo, hi int) bool {
	if field == "*" {
		return true
	}

	for _, part := range strings.Split(field, ",") {
		step := 1
		if idx := strings.Index(part, "/"); idx >= 0 {
			stepStr := part[idx+1:]
			part = part[:idx]
			s, err := strconv.Atoi(stepStr)
			if err != nil {
				continue
			}
			step = s
		}

		if part == "*" {
			// */N
			if step > 0 && (value-lo)%step == 0 {
				return true
			}
		} else if idx := strings.Index(part, "-"); idx >= 0 {
			// Range: N-M
			start, err1 := strconv.Atoi(part[:idx])
			end, err2 := strconv.Atoi(part[idx+1:])
			if err1 != nil || err2 != nil {
				continue
			}
			if value >= start && value <= end && (value-start)%step == 0 {
				return true
			}
		} else {
			// Exact value
			n, err := strconv.Atoi(part)
			if err != nil {
				continue
			}
			if n == value {
				return true
			}
		}
	}
	return false
}

// computeJitter returns a small offset (1-4 minutes) if the cron targets :00 or :30.
func computeJitter(cronExpr string) int {
	fields := strings.Fields(cronExpr)
	if len(fields) < 1 {
		return 0
	}
	minuteVal, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0
	}
	if jitterMinutes[minuteVal] {
		h := fnv.New32a()
		h.Write([]byte(cronExpr))
		return int(h.Sum32()%uint32(JitterOffsetMax)) + 1
	}
	return 0
}
