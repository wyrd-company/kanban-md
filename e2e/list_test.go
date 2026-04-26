package e2e_test

import (
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// List tests
// ---------------------------------------------------------------------------

func TestListEmpty(t *testing.T) {
	kanbanDir := initBoard(t)

	var tasks []taskJSON
	runKanbanJSON(t, kanbanDir, &tasks, "list")
	if len(tasks) != 0 {
		t.Errorf("list returned %d tasks, want 0", len(tasks))
	}

	// Table output writes "No tasks found." to stderr.
	r := runKanban(t, kanbanDir, "--table", "list")
	if !strings.Contains(r.stderr, "No tasks found.") {
		t.Errorf("stderr = %q, want 'No tasks found.'", r.stderr)
	}
}

func TestListFilters(t *testing.T) {
	kanbanDir := initBoard(t)

	mustCreateTask(t, kanbanDir, "Backend API", "--status", statusTodo, "--priority", "high",
		"--assignee", assigneeAlice, "--tags", "backend,api")
	mustCreateTask(t, kanbanDir, "Frontend UI", "--status", "in-progress", "--priority", "medium",
		"--assignee", "bob", "--tags", "frontend")
	mustCreateTask(t, kanbanDir, "Database Migration", "--status", statusTodo, "--priority", "critical",
		"--assignee", assigneeAlice, "--tags", "backend,database")
	mustCreateTask(t, kanbanDir, "Docs Update", "--status", "done", "--priority", "low",
		"--tags", "docs")

	tests := []struct {
		name    string
		args    []string
		wantIDs []int
	}{
		{"status todo", []string{"--status", statusTodo}, []int{1, 3}},
		{"multiple statuses", []string{"--status", "todo,done"}, []int{1, 3, 4}},
		{"assignee alice", []string{"--assignee", assigneeAlice}, []int{1, 3}},
		{"tag backend", []string{"--tag", "backend"}, []int{1, 3}},
		{"priority high", []string{"--priority", "high"}, []int{1}},
		{"status+assignee", []string{"--status", statusTodo, "--assignee", assigneeAlice}, []int{1, 3}},
		{"assignee+tag", []string{"--assignee", assigneeAlice, "--tag", "api"}, []int{1}},
		{"no match", []string{"--assignee", "nobody"}, []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"list"}, tt.args...)
			var tasks []taskJSON
			runKanbanJSON(t, kanbanDir, &tasks, args...)

			gotIDs := make([]int, len(tasks))
			for i, task := range tasks {
				gotIDs[i] = task.ID
			}

			if len(gotIDs) != len(tt.wantIDs) {
				t.Fatalf("got IDs %v, want %v", gotIDs, tt.wantIDs)
			}
			for i, id := range gotIDs {
				if id != tt.wantIDs[i] {
					t.Errorf("task[%d].ID = %d, want %d", i, id, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestListSortAndLimit(t *testing.T) {
	kanbanDir := initBoard(t)

	mustCreateTask(t, kanbanDir, "C task", "--priority", "low")
	mustCreateTask(t, kanbanDir, "A task", "--priority", "high")
	mustCreateTask(t, kanbanDir, "B task", "--priority", "critical")

	tests := []struct {
		name    string
		args    []string
		wantIDs []int
	}{
		{"default sort by id", nil, []int{1, 2, 3}},
		{"sort id reverse", []string{"--sort", "id", "--reverse"}, []int{3, 2, 1}},
		{"sort by priority", []string{"--sort", "priority"}, []int{1, 2, 3}},
		{"limit 2", []string{"--limit", "2"}, []int{1, 2}},
		{"reverse + limit 1", []string{"--sort", "id", "--reverse", "--limit", "1"}, []int{3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"list"}, tt.args...)
			var tasks []taskJSON
			runKanbanJSON(t, kanbanDir, &tasks, args...)

			gotIDs := make([]int, len(tasks))
			for i, task := range tasks {
				gotIDs[i] = task.ID
			}

			if len(gotIDs) != len(tt.wantIDs) {
				t.Fatalf("got IDs %v, want %v", gotIDs, tt.wantIDs)
			}
			for i, id := range gotIDs {
				if id != tt.wantIDs[i] {
					t.Errorf("position %d: got ID %d, want %d", i, id, tt.wantIDs[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Show tests
// ---------------------------------------------------------------------------

func TestListByParent(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Parent")
	mustCreateTask(t, kanbanDir, "Child A", "--parent", "1")
	mustCreateTask(t, kanbanDir, "Child B", "--parent", "1")
	mustCreateTask(t, kanbanDir, "Orphan")

	var tasks []map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &tasks, "list", "--parent", "1")
	if r.exitCode != 0 {
		t.Fatalf("list --parent failed: %s", r.stderr)
	}
	if len(tasks) != 2 {
		t.Errorf("got %d tasks, want 2 with parent 1", len(tasks))
	}
}

func TestListUnblocked(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Dep task")                          // #1
	mustCreateTask(t, kanbanDir, "Depends on 1", "--depends-on", "1") // #2
	mustCreateTask(t, kanbanDir, "No deps")                           // #3

	// Task 1 is in backlog (not done), so task 2 is blocked by deps.
	var tasks []map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &tasks, "list", "--unblocked")
	if r.exitCode != 0 {
		t.Fatalf("list --unblocked failed: %s", r.stderr)
	}
	// Only tasks 1 and 3 should appear (no unsatisfied deps).
	if len(tasks) != 2 {
		t.Errorf("got %d unblocked tasks, want 2", len(tasks))
	}
}

func TestListUnblockedAfterDepDone(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Dep task")                          // #1
	mustCreateTask(t, kanbanDir, "Depends on 1", "--depends-on", "1") // #2

	// Move dep to done.
	runKanban(t, kanbanDir, "--json", "move", "1", "done")

	var tasks []map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &tasks, "list", "--unblocked")
	if r.exitCode != 0 {
		t.Fatalf("list --unblocked failed: %s", r.stderr)
	}
	// Both tasks should now be unblocked.
	if len(tasks) != 2 {
		t.Errorf("got %d unblocked tasks, want 2 (dep satisfied)", len(tasks))
	}
}

func TestListUnblockedWithMissingDependencyFile(t *testing.T) {
	kanbanDir := initBoard(t)
	dep := mustCreateTask(t, kanbanDir, "Dep task")
	mustCreateTask(t, kanbanDir, "Depends on missing", "--depends-on", "1")

	removeTaskFromRef(t, kanbanDir, dep.ID)

	var tasks []taskJSON
	r := runKanbanJSON(t, kanbanDir, &tasks, "list", "--unblocked")
	if r.exitCode != 0 {
		t.Fatalf("list --unblocked failed: %s", r.stderr)
	}
	if len(tasks) != 1 {
		t.Fatalf("got %d unblocked tasks, want 1", len(tasks))
	}
	if tasks[0].ID != 2 {
		t.Errorf("unblocked task ID = %d, want 2", tasks[0].ID)
	}
}

func TestListBlocked(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Normal task")
	mustCreateTask(t, kanbanDir, "Blocked task")
	runKanban(t, kanbanDir, "--json", "edit", "2", "--block", "stuck on dep")

	var tasks []struct {
		taskJSON
		Blocked bool `json:"blocked"`
	}
	runKanbanJSON(t, kanbanDir, &tasks, "list", "--blocked")
	if len(tasks) != 1 {
		t.Fatalf("got %d blocked tasks, want 1", len(tasks))
	}
	if tasks[0].ID != 2 {
		t.Errorf("blocked task ID = %d, want 2", tasks[0].ID)
	}
}

func TestListNotBlocked(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Normal task")
	mustCreateTask(t, kanbanDir, "Blocked task")
	runKanban(t, kanbanDir, "--json", "edit", "2", "--block", "stuck")

	var tasks []struct {
		taskJSON
		Blocked bool `json:"blocked"`
	}
	runKanbanJSON(t, kanbanDir, &tasks, "list", "--not-blocked")
	if len(tasks) != 1 {
		t.Fatalf("got %d not-blocked tasks, want 1", len(tasks))
	}
	if tasks[0].ID != 1 {
		t.Errorf("not-blocked task ID = %d, want 1", tasks[0].ID)
	}
}

func TestListUnclaimedFilter(t *testing.T) {
	kanbanDir := initBoard(t)

	// Task 1: claimed (should be excluded from --unclaimed list).
	writeTaskFile(t, kanbanDir, 1, `---
id: 1
title: Claimed task
status: todo
priority: high
created: 2026-01-01T00:00:00Z
updated: 2026-01-01T00:00:00Z
claimed_by: agent-alpha
claimed_at: 2099-01-01T00:00:00Z
---
`)
	// Task 2: unclaimed.
	writeTaskFile(t, kanbanDir, 2, `---
id: 2
title: Free task
status: todo
priority: medium
created: 2026-01-01T00:00:00Z
updated: 2026-01-01T00:00:00Z
---
`)
	// Task 3: expired claim (should appear as unclaimed).
	writeTaskFile(t, kanbanDir, 3, `---
id: 3
title: Expired claim task
status: todo
priority: low
created: 2026-01-01T00:00:00Z
updated: 2026-01-01T00:00:00Z
claimed_by: agent-old
claimed_at: 2020-01-01T00:00:00Z
---
`)
	bumpNextID(t, kanbanDir, 4)

	var tasks []taskJSON
	r := runKanbanJSON(t, kanbanDir, &tasks, "list", "--unclaimed")
	if r.exitCode != 0 {
		t.Fatalf("list --unclaimed failed (exit %d): %s", r.exitCode, r.stderr)
	}

	if len(tasks) != 2 {
		t.Errorf("got %d tasks, want 2 (unclaimed + expired)", len(tasks))
		for _, tk := range tasks {
			t.Logf("  got task #%d %q", tk.ID, tk.Title)
		}
	}

	ids := make(map[int]bool, len(tasks))
	for _, tk := range tasks {
		ids[tk.ID] = true
	}
	if ids[1] {
		t.Error("task #1 (active claim) should NOT appear in --unclaimed list")
	}
	if !ids[2] {
		t.Error("task #2 (unclaimed) should appear in --unclaimed list")
	}
	if !ids[3] {
		t.Error("task #3 (expired claim) should appear in --unclaimed list")
	}
}

func TestListClaimedByFilter(t *testing.T) {
	kanbanDir := initBoard(t)

	writeTaskFile(t, kanbanDir, 1, `---
id: 1
title: Agent alpha task
status: todo
priority: high
created: 2026-01-01T00:00:00Z
updated: 2026-01-01T00:00:00Z
claimed_by: agent-alpha
claimed_at: 2099-01-01T00:00:00Z
---
`)
	writeTaskFile(t, kanbanDir, 2, `---
id: 2
title: Agent beta task
status: todo
priority: medium
created: 2026-01-01T00:00:00Z
updated: 2026-01-01T00:00:00Z
claimed_by: agent-beta
claimed_at: 2099-01-01T00:00:00Z
---
`)
	writeTaskFile(t, kanbanDir, 3, `---
id: 3
title: Unclaimed task
status: todo
priority: low
created: 2026-01-01T00:00:00Z
updated: 2026-01-01T00:00:00Z
---
`)
	bumpNextID(t, kanbanDir, 4)

	var tasks []taskJSON
	r := runKanbanJSON(t, kanbanDir, &tasks, "list", "--claimed-by", "agent-alpha")
	if r.exitCode != 0 {
		t.Fatalf("list --claimed-by failed (exit %d): %s", r.exitCode, r.stderr)
	}

	if len(tasks) != 1 {
		t.Errorf("got %d tasks, want 1", len(tasks))
	}
	if len(tasks) > 0 && tasks[0].ID != 1 {
		t.Errorf("got task #%d, want #1", tasks[0].ID)
	}
}

// ---------------------------------------------------------------------------
// Read command coverage tests (list, show, board, log, metrics)
// ---------------------------------------------------------------------------

func TestListGroupByStatus(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Task A", "--status", statusTodo)
	mustCreateTask(t, kanbanDir, "Task B", "--status", statusTodo)
	taskC := mustCreateTask(t, kanbanDir, "Task C")
	runKanban(t, kanbanDir, "move", strconv.Itoa(taskC.ID), "in-progress", "--claim", claimTestAgent)

	// Table output with --group-by status.
	r := runKanban(t, kanbanDir, "list", "--group-by", "status")
	if r.exitCode != 0 {
		t.Fatalf("list --group-by status failed (exit %d): %s", r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stdout, statusTodo) {
		t.Error("expected group header 'todo' in output")
	}

	// JSON output with --group-by status.
	var grouped struct {
		Groups []struct {
			Key string `json:"key"`
		} `json:"groups"`
	}
	r = runKanbanJSON(t, kanbanDir, &grouped, "list", "--group-by", "status")
	if r.exitCode != 0 {
		t.Fatalf("list --group-by --json failed (exit %d): %s", r.exitCode, r.stderr)
	}
	foundTodo := false
	for _, g := range grouped.Groups {
		if g.Key == statusTodo {
			foundTodo = true
		}
	}
	if !foundTodo {
		t.Error("JSON output should have group with key 'todo'")
	}
}

func TestListGroupByPriority(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "High task", "--priority", "high")
	mustCreateTask(t, kanbanDir, "Low task", "--priority", "low")

	r := runKanban(t, kanbanDir, "list", "--group-by", "priority")
	if r.exitCode != 0 {
		t.Fatalf("list --group-by priority failed (exit %d): %s", r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stdout, "high") {
		t.Error("expected group header 'high' in output")
	}
}

func TestListGroupByInvalid(t *testing.T) {
	kanbanDir := initBoard(t)

	errResp := runKanbanJSONError(t, kanbanDir, "list", "--group-by", "invalid-field")
	if errResp.Code != "INVALID_GROUP_BY" {
		t.Errorf("error code = %q, want INVALID_GROUP_BY", errResp.Code)
	}
}

func TestListCompactOutput(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Compact list task")

	r := runKanban(t, kanbanDir, "list", "--compact")
	if r.exitCode != 0 {
		t.Fatalf("list --compact failed (exit %d): %s", r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stdout, "Compact list task") {
		t.Error("compact list output should contain task title")
	}
}
