package e2e_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Edit tests
// ---------------------------------------------------------------------------

func TestEditFields(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Edit target", "--tags", "original")

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, task taskJSON)
	}{
		{
			name: "change status",
			args: []string{"--status", statusTodo},
			check: func(t *testing.T, task taskJSON) {
				t.Helper()
				if task.Status != statusTodo {
					t.Errorf("Status = %q, want %q", task.Status, statusTodo)
				}
			},
		},
		{
			name: "change priority",
			args: []string{"--priority", "high"},
			check: func(t *testing.T, task taskJSON) {
				t.Helper()
				if task.Priority != priorityHigh {
					t.Errorf("Priority = %q, want %q", task.Priority, priorityHigh)
				}
			},
		},
		{
			name: "add tag",
			args: []string{"--add-tag", "newtag"},
			check: func(t *testing.T, task taskJSON) {
				t.Helper()
				found := false
				for _, tag := range task.Tags {
					if tag == "newtag" {
						found = true
					}
				}
				if !found {
					t.Errorf("Tags %v missing %q", task.Tags, "newtag")
				}
			},
		},
		{
			name: "set due date",
			args: []string{"--due", "2026-06-15"},
			check: func(t *testing.T, task taskJSON) {
				t.Helper()
				if task.Due != "2026-06-15" {
					t.Errorf("Due = %q, want %q", task.Due, "2026-06-15")
				}
			},
		},
		{
			name: "set body",
			args: []string{"--body", "Updated body content"},
			check: func(t *testing.T, task taskJSON) {
				t.Helper()
				if !strings.Contains(task.Body, "Updated body content") {
					t.Errorf("Body = %q, want %q", task.Body, "Updated body content")
				}
			},
		},
	}

	// These run sequentially on the same task — each edit builds on previous state.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editArgs := append([]string{"edit", "1"}, tt.args...)
			var task taskJSON
			r := runKanbanJSON(t, kanbanDir, &task, editArgs...)
			if r.exitCode != 0 {
				t.Fatalf("edit failed: %s", r.stderr)
			}
			tt.check(t, task)
		})
	}
}

func TestEditTitleRename(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Original title")

	var updated taskJSON
	runKanbanJSON(t, kanbanDir, &updated, "edit", "1", "--title", "New title")

	if updated.Title != "New title" {
		t.Errorf("Title = %q, want %q", updated.Title, "New title")
	}

	if !strings.Contains(filepath.Base(updated.File), "new-title") {
		t.Errorf("filename %q missing 'new-title'", filepath.Base(updated.File))
	}
}

func TestEditNoChanges(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Stable task")

	errResp := runKanbanJSONError(t, kanbanDir, "edit", "1")
	if errResp.Code != "NO_CHANGES" {
		t.Errorf("code = %q, want NO_CHANGES", errResp.Code)
	}
	if !strings.Contains(errResp.Error, "no changes") {
		t.Errorf("error = %q, want 'no changes'", errResp.Error)
	}
}

func TestEditClearDue(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Dated task", "--due", "2026-03-15")

	var task taskJSON
	runKanbanJSON(t, kanbanDir, &task, "edit", "1", "--clear-due")

	if task.Due != "" {
		t.Errorf("Due = %q, want empty (cleared)", task.Due)
	}
}

func TestEditAppendBody(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Append target", "--body", "initial body")

	var task taskJSON
	r := runKanbanJSON(t, kanbanDir, &task, "edit", "1", "--append-body", "appended note")
	if r.exitCode != 0 {
		t.Fatalf("edit failed: %s", r.stderr)
	}
	if !strings.Contains(task.Body, "initial body") {
		t.Errorf("Body should contain original text, got %q", task.Body)
	}
	if !strings.Contains(task.Body, "appended note") {
		t.Errorf("Body should contain appended text, got %q", task.Body)
	}
}

func TestEditAppendBody_ToEmptyBody(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Empty body target")

	var task taskJSON
	r := runKanbanJSON(t, kanbanDir, &task, "edit", "1", "--append-body", "first note")
	if r.exitCode != 0 {
		t.Fatalf("edit failed: %s", r.stderr)
	}
	if task.Body != "first note" {
		t.Errorf("Body = %q, want %q", task.Body, "first note")
	}
}

func TestEditAppendBody_WithTimestamp(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Timestamp target", "--body", "existing")

	var task taskJSON
	r := runKanbanJSON(t, kanbanDir, &task, "edit", "1", "-a", "progress update", "-t")
	if r.exitCode != 0 {
		t.Fatalf("edit failed: %s", r.stderr)
	}
	if !strings.Contains(task.Body, "existing") {
		t.Errorf("Body should contain original text, got %q", task.Body)
	}
	if !strings.Contains(task.Body, "[[") || !strings.Contains(task.Body, "]]") {
		t.Errorf("Body should contain timestamp markers, got %q", task.Body)
	}
	if !strings.Contains(task.Body, "progress update") {
		t.Errorf("Body should contain appended text, got %q", task.Body)
	}
}

func TestEditAppendBody_MultipleAppends(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Multi-append target")

	var task taskJSON
	runKanbanJSON(t, kanbanDir, &task, "edit", "1", "--append-body", "note 1")
	runKanbanJSON(t, kanbanDir, &task, "edit", "1", "--append-body", "note 2")
	runKanbanJSON(t, kanbanDir, &task, "edit", "1", "--append-body", "note 3")

	if !strings.Contains(task.Body, "note 1") {
		t.Errorf("Body should contain note 1, got %q", task.Body)
	}
	if !strings.Contains(task.Body, "note 2") {
		t.Errorf("Body should contain note 2, got %q", task.Body)
	}
	if !strings.Contains(task.Body, "note 3") {
		t.Errorf("Body should contain note 3, got %q", task.Body)
	}
}

func TestEditBodyAndAppendBodyConflict(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Conflict target")

	errResp := runKanbanJSONError(t, kanbanDir, "edit", "1", "--body", "replace", "--append-body", "append")
	if errResp.Code != codeStatusConflict {
		t.Errorf("code = %q, want STATUS_CONFLICT", errResp.Code)
	}
}

// ---------------------------------------------------------------------------
// Move tests
// ---------------------------------------------------------------------------

func TestEditStatusRespectsWIPLimit(t *testing.T) {
	kanbanDir := initBoardWithWIP(t, 1)

	mustCreateTask(t, kanbanDir, "Task A", "--status", "in-progress")
	mustCreateTask(t, kanbanDir, "Task B")

	// Edit task B status to in-progress should fail.
	errResp := runKanbanJSONError(t, kanbanDir, "edit", "2", "--status", "in-progress", "--claim", claimTestAgent)
	if errResp.Code != codeWIPLimitExceeded {
		t.Errorf("code = %q, want WIP_LIMIT_EXCEEDED", errResp.Code)
	}
}

func TestEditStartedManualBackfill(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Task A")

	var edited map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &edited, "edit", "1", "--started", "2026-01-15")
	if r.exitCode != 0 {
		t.Fatalf("edit --started failed: %s", r.stderr)
	}
	started, ok := edited["started"].(string)
	if !ok || !strings.HasPrefix(started, "2026-01-15") {
		t.Errorf("started = %v, want prefix 2026-01-15", edited["started"])
	}
}

func TestEditCompletedManualBackfill(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Task A")

	var edited map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &edited, "edit", "1", "--completed", "2026-02-01")
	if r.exitCode != 0 {
		t.Fatalf("edit --completed failed: %s", r.stderr)
	}
	completed, ok := edited["completed"].(string)
	if !ok || !strings.HasPrefix(completed, "2026-02-01") {
		t.Errorf("completed = %v, want prefix 2026-02-01", edited["completed"])
	}
}

func TestEditClearStarted(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Task A")
	runKanban(t, kanbanDir, "--json", "edit", "1", "--started", "2026-01-15")

	var edited map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &edited, "edit", "1", "--clear-started")
	if r.exitCode != 0 {
		t.Fatalf("edit --clear-started failed: %s", r.stderr)
	}
	if edited["started"] != nil {
		t.Errorf("started = %v, want nil after clear", edited["started"])
	}
}

func TestEditClearCompleted(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Task A")
	runKanban(t, kanbanDir, "--json", "edit", "1", "--completed", "2026-02-01")

	var edited map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &edited, "edit", "1", "--clear-completed")
	if r.exitCode != 0 {
		t.Fatalf("edit --clear-completed failed: %s", r.stderr)
	}
	if edited["completed"] != nil {
		t.Errorf("completed = %v, want nil after clear", edited["completed"])
	}
}

func TestEditStartedAndClearStartedConflict(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Task A")

	errResp := runKanbanJSONError(t, kanbanDir, "edit", "1", "--started", "2026-01-15", "--clear-started")
	if errResp.Code != codeStatusConflict {
		t.Errorf("code = %q, want STATUS_CONFLICT", errResp.Code)
	}
	if !strings.Contains(errResp.Error, "cannot use") {
		t.Errorf("error = %q, want conflict error", errResp.Error)
	}
}
