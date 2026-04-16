// Package s03_planning adds session-level planning on top of the s02 tool agent.
//
// Key insight: the plan is a "working memory" tool — it doesn't need persistence,
// just session-scoped state that the LLM can read and rewrite.
package s03_planning

import (
	"fmt"
	"strings"
)

// PlanStatus represents the state of a plan item.
type PlanStatus string

const (
	StatusPending    PlanStatus = "pending"
	StatusInProgress PlanStatus = "in_progress"
	StatusCompleted  PlanStatus = "completed"
)

// MaxPlanItems is the maximum number of items allowed in a session plan.
const MaxPlanItems = 12

// PlanItem is one step in the session plan.
type PlanItem struct {
	Content    string     `json:"content"`
	Status     PlanStatus `json:"status"`
	ActiveForm string     `json:"activeForm,omitempty"`
}

// PlanManager manages the session plan and tracks update freshness.
type PlanManager struct {
	items             []PlanItem
	roundsSinceUpdate int
	reminderInterval  int
}

// NewPlanManager creates a PlanManager with the given reminder interval.
func NewPlanManager(reminderInterval int) *PlanManager {
	return &PlanManager{
		reminderInterval: reminderInterval,
	}
}

// Update replaces the entire plan with validated items. Resets the round counter.
func (pm *PlanManager) Update(rawItems []map[string]any) (string, error) {
	if len(rawItems) > MaxPlanItems {
		return "", fmt.Errorf("keep the session plan short (max %d items)", MaxPlanItems)
	}

	items := make([]PlanItem, 0, len(rawItems))
	inProgressCount := 0

	for i, raw := range rawItems {
		content := strings.TrimSpace(fmt.Sprintf("%v", raw["content"]))
		if content == "" {
			return "", fmt.Errorf("item %d: content required", i)
		}

		statusStr := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", raw["status"])))
		if statusStr == "" {
			statusStr = "pending"
		}
		status := PlanStatus(statusStr)
		if status != StatusPending && status != StatusInProgress && status != StatusCompleted {
			return "", fmt.Errorf("item %d: invalid status %q", i, statusStr)
		}
		if status == StatusInProgress {
			inProgressCount++
		}

		activeForm := ""
		if af, ok := raw["activeForm"]; ok {
			activeForm = strings.TrimSpace(fmt.Sprintf("%v", af))
		}

		items = append(items, PlanItem{
			Content:    content,
			Status:     status,
			ActiveForm: activeForm,
		})
	}

	if inProgressCount > 1 {
		return "", fmt.Errorf("only one plan item can be in_progress")
	}

	pm.items = items
	pm.roundsSinceUpdate = 0
	return pm.Render(), nil
}

// NoteRoundWithoutUpdate increments the stale-round counter.
func (pm *PlanManager) NoteRoundWithoutUpdate() {
	pm.roundsSinceUpdate++
}

// Reminder returns a nudge message if the plan hasn't been updated recently,
// or empty string if no reminder is needed.
func (pm *PlanManager) Reminder() string {
	if len(pm.items) == 0 {
		return ""
	}
	if pm.roundsSinceUpdate < pm.reminderInterval {
		return ""
	}
	return "<reminder>Refresh your current plan before continuing.</reminder>"
}

// Render formats the plan as a human-readable checklist.
func (pm *PlanManager) Render() string {
	if len(pm.items) == 0 {
		return "No session plan yet."
	}

	markers := map[PlanStatus]string{
		StatusPending:    "[ ]",
		StatusInProgress: "[>]",
		StatusCompleted:  "[x]",
	}

	var lines []string
	completed := 0
	for _, item := range pm.items {
		line := fmt.Sprintf("%s %s", markers[item.Status], item.Content)
		if item.Status == StatusInProgress && item.ActiveForm != "" {
			line += fmt.Sprintf(" (%s)", item.ActiveForm)
		}
		lines = append(lines, line)
		if item.Status == StatusCompleted {
			completed++
		}
	}
	lines = append(lines, fmt.Sprintf("\n(%d/%d completed)", completed, len(pm.items)))
	return strings.Join(lines, "\n")
}

// HasItems returns true if there is at least one plan item.
func (pm *PlanManager) HasItems() bool {
	return len(pm.items) > 0
}
