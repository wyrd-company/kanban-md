package e2e_test

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Cross-cutting tests
// ---------------------------------------------------------------------------

func TestNoInitErrors(t *testing.T) {
	dir := t.TempDir() // empty, no board

	commands := []struct {
		name string
		args []string
	}{
		{"create", []string{"create", "Task"}},
		{"list", []string{"list"}},
		{"show", []string{"show", "1"}},
		{"edit", []string{"edit", "1", "--title", "New"}},
		{"move", []string{"move", "1", "done"}},
		{"delete", []string{"delete", "1", "--yes"}},
	}

	for _, tt := range commands {
		t.Run(tt.name, func(t *testing.T) {
			r := runKanban(t, dir, tt.args...)
			if r.exitCode == 0 {
				t.Errorf("%s succeeded without an initialized board", tt.name)
			}
			if !strings.Contains(r.stderr, "no kanban board found") {
				t.Errorf("stderr = %q, want 'no kanban board found'", r.stderr)
			}
		})
	}
}

func TestCommandAliases(t *testing.T) {
	kanbanDir := initBoard(t)

	// add = create
	var task taskJSON
	r := runKanbanJSON(t, kanbanDir, &task, "add", "Aliased task")
	if r.exitCode != 0 {
		t.Fatalf("'add' alias failed: %s", r.stderr)
	}
	if task.Title != "Aliased task" {
		t.Errorf("Title = %q, want %q", task.Title, "Aliased task")
	}

	// ls = list
	var tasks []taskJSON
	r = runKanbanJSON(t, kanbanDir, &tasks, "ls")
	if r.exitCode != 0 {
		t.Fatalf("'ls' alias failed: %s", r.stderr)
	}
	if len(tasks) != 1 {
		t.Errorf("ls returned %d tasks, want 1", len(tasks))
	}

	// rm = delete
	var deleted map[string]interface{}
	r = runKanbanJSON(t, kanbanDir, &deleted, "rm", "1", "--yes")
	if r.exitCode != 0 {
		t.Fatalf("'rm' alias failed: %s", r.stderr)
	}
}

// ---------------------------------------------------------------------------
// Workflow & edge case tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Workflow & edge case tests
// ---------------------------------------------------------------------------

func TestFullLifecycle(t *testing.T) {
	kanbanDir := initBoard(t)

	// Create.
	task := mustCreateTask(t, kanbanDir, "Lifecycle task",
		"--priority", "high", "--assignee", assigneeAlice)
	if task.ID != 1 {
		t.Fatalf("create: ID = %d, want 1", task.ID)
	}

	// List.
	var tasks []taskJSON
	runKanbanJSON(t, kanbanDir, &tasks, "list")
	if len(tasks) != 1 {
		t.Fatalf("list: got %d tasks, want 1", len(tasks))
	}

	// Show.
	var shown taskJSON
	runKanbanJSON(t, kanbanDir, &shown, "show", "1")
	if shown.Assignee != assigneeAlice {
		t.Errorf("show: Assignee = %q, want %q", shown.Assignee, assigneeAlice)
	}

	// Edit.
	var edited taskJSON
	runKanbanJSON(t, kanbanDir, &edited, "edit", "1", "--priority", "critical")
	if edited.Priority != "critical" {
		t.Errorf("edit: Priority = %q, want %q", edited.Priority, "critical")
	}

	// Move.
	var moved taskJSON
	runKanbanJSON(t, kanbanDir, &moved, "move", "1", "--next")
	if moved.Status != statusTodo {
		t.Errorf("move: Status = %q, want %q", moved.Status, statusTodo)
	}

	// Delete.
	var deleted map[string]interface{}
	runKanbanJSON(t, kanbanDir, &deleted, "delete", "1", "--yes")
	if deleted["status"] != statusDeleted {
		t.Errorf("delete: status = %v, want %q", deleted["status"], statusDeleted)
	}

	// List (empty).
	runKanbanJSON(t, kanbanDir, &tasks, "list")
	if len(tasks) != 0 {
		t.Errorf("list after delete: got %d tasks, want 0", len(tasks))
	}
}

func TestCustomStatusesWorkflow(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	kanbanDir := filepath.Join(dir, "kanban")

	var initResult map[string]string
	runKanbanJSON(t, kanbanDir, &initResult, "init", "--statuses", "open,wip,closed")

	// Default status is first: "open".
	task := mustCreateTask(t, kanbanDir, "Custom status task")
	if task.Status != "open" {
		t.Errorf("default status = %q, want %q", task.Status, "open")
	}

	// Move next: open -> wip.
	var moved taskJSON
	runKanbanJSON(t, kanbanDir, &moved, "move", "1", "--next")
	if moved.Status != "wip" {
		t.Errorf("after --next: status = %q, want %q", moved.Status, "wip")
	}

	// Move next: wip -> closed.
	runKanbanJSON(t, kanbanDir, &moved, "move", "1", "--next")
	if moved.Status != "closed" {
		t.Errorf("after second --next: status = %q, want %q", moved.Status, "closed")
	}

	// --next at last fails.
	r := runKanban(t, kanbanDir, "--json", "move", "1", "--next")
	if r.exitCode == 0 {
		t.Error("expected failure for --next at last status")
	}

	// Old default statuses rejected.
	r = runKanban(t, kanbanDir, "--json", "create", "Bad status", "--status", "backlog")
	if r.exitCode == 0 {
		t.Error("expected failure for status 'backlog' not in custom statuses")
	}
}

func TestTagOperations(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Tagged task", "--tags", "alpha,beta")

	var task taskJSON

	// Add new tag.
	runKanbanJSON(t, kanbanDir, &task, "edit", "1", "--add-tag", "gamma")
	if len(task.Tags) != 3 {
		t.Fatalf("after add: Tags = %v, want 3 tags", task.Tags)
	}

	// Add duplicate (should not duplicate).
	runKanbanJSON(t, kanbanDir, &task, "edit", "1", "--add-tag", "alpha")
	if len(task.Tags) != 3 {
		t.Errorf("after adding duplicate: Tags = %v, want 3 tags still", task.Tags)
	}

	// Remove tag.
	runKanbanJSON(t, kanbanDir, &task, "edit", "1", "--remove-tag", "beta")
	if len(task.Tags) != 2 {
		t.Errorf("after remove: Tags = %v, want 2 tags", task.Tags)
	}
	for _, tag := range task.Tags {
		if tag == "beta" {
			t.Error("removed tag 'beta' still present")
		}
	}

	// Remove non-existent tag (should succeed).
	r := runKanbanJSON(t, kanbanDir, &task, "edit", "1", "--remove-tag", "nonexistent")
	if r.exitCode != 0 {
		t.Errorf("removing non-existent tag failed: %s", r.stderr)
	}
}

func TestDueDateLifecycle(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Due task")

	// Set due date.
	var set taskJSON
	runKanbanJSON(t, kanbanDir, &set, "edit", "1", "--due", "2026-04-01")
	if set.Due != "2026-04-01" {
		t.Errorf("Due = %q, want %q", set.Due, "2026-04-01")
	}

	// Clear due date — use fresh struct so omitted fields don't carry over.
	var cleared taskJSON
	runKanbanJSON(t, kanbanDir, &cleared, "edit", "1", "--clear-due")
	if cleared.Due != "" {
		t.Errorf("Due = %q after clear, want empty", cleared.Due)
	}

	// Re-set due date.
	var reset taskJSON
	runKanbanJSON(t, kanbanDir, &reset, "edit", "1", "--due", "2026-05-01")
	if reset.Due != "2026-05-01" {
		t.Errorf("Due = %q, want %q", reset.Due, "2026-05-01")
	}
}

func TestSortByDueWithNilValues(t *testing.T) {
	kanbanDir := initBoard(t)

	mustCreateTask(t, kanbanDir, "No due")
	mustCreateTask(t, kanbanDir, "Late due", "--due", "2026-12-31")
	mustCreateTask(t, kanbanDir, "Early due", "--due", "2026-01-01")

	var tasks []taskJSON
	runKanbanJSON(t, kanbanDir, &tasks, "list", "--sort", "due")

	// Early first, late second, nil last.
	wantIDs := []int{3, 2, 1}
	if len(tasks) != len(wantIDs) {
		t.Fatalf("got %d tasks, want %d", len(tasks), len(wantIDs))
	}
	for i, want := range wantIDs {
		if tasks[i].ID != want {
			t.Errorf("position %d: ID = %d, want %d", i, tasks[i].ID, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Output format tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Slug / output tests
// ---------------------------------------------------------------------------

func TestLongTitleSlugTruncation(t *testing.T) {
	kanbanDir := initBoard(t)

	longTitle := "This is a very long title that should be truncated at a word boundary to fit within the slug limit"
	task := mustCreateTask(t, kanbanDir, longTitle)

	basename := filepath.Base(task.File)
	// Remove "001-" prefix and ".md" suffix to get slug.
	slug := strings.TrimSuffix(strings.TrimPrefix(basename, "001-"), ".md")
	if len(slug) > 50 {
		t.Errorf("slug length = %d, want <= 50: %q", len(slug), slug)
	}
}

// ---------------------------------------------------------------------------
// Config command tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Concurrent create — duplicate ID prevention
// ---------------------------------------------------------------------------

func TestConcurrentCreateUniqueIDs(t *testing.T) {
	kanbanDir := initBoard(t)

	const n = 10
	type createResult struct {
		task taskJSON
		err  error
	}
	results := make(chan createResult, n)

	// Launch n concurrent create processes.
	for i := range n {
		go func(idx int) {
			title := fmt.Sprintf("Concurrent task %d", idx)
			args := []string{"--dir", kanbanDir, "--json", "create", title}
			cmd := exec.Command(binPath, args...) //nolint:gosec,noctx // e2e test
			out, err := cmd.Output()
			if err != nil {
				results <- createResult{err: fmt.Errorf("create %d failed: %w", idx, err)}
				return
			}
			var tk taskJSON
			if err := json.Unmarshal(out, &tk); err != nil {
				results <- createResult{err: fmt.Errorf("parse %d: %w", idx, err)}
				return
			}
			results <- createResult{task: tk}
		}(i)
	}

	ids := make(map[int]bool, n)
	for range n {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		if ids[r.task.ID] {
			t.Errorf("duplicate ID %d", r.task.ID)
		}
		ids[r.task.ID] = true
	}

	if len(ids) != n {
		t.Errorf("expected %d unique IDs, got %d", n, len(ids))
	}
}
