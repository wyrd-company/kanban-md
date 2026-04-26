package e2e_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Move tests
// ---------------------------------------------------------------------------

func TestMoveDirectStatus(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Movable task")

	var task taskJSON
	runKanbanJSON(t, kanbanDir, &task, "move", "1", "in-progress", "--claim", claimTestAgent)

	if task.Status != statusInProgress {
		t.Errorf("Status = %q, want %q", task.Status, "in-progress")
	}
}

func TestMoveNextPrev(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Walk task") // starts at "backlog"

	var task taskJSON

	// backlog -> todo
	runKanbanJSON(t, kanbanDir, &task, "move", "1", "--next")
	if task.Status != statusTodo {
		t.Errorf("after --next: Status = %q, want %q", task.Status, statusTodo)
	}

	// todo -> in-progress (requires --claim)
	runKanbanJSON(t, kanbanDir, &task, "move", "1", "--next", "--claim", claimTestAgent)
	if task.Status != statusInProgress {
		t.Errorf("after second --next: Status = %q, want %q", task.Status, "in-progress")
	}

	// in-progress -> todo (--claim needed because task is now claimed)
	runKanbanJSON(t, kanbanDir, &task, "move", "1", "--prev", "--claim", claimTestAgent)
	if task.Status != statusTodo {
		t.Errorf("after --prev: Status = %q, want %q", task.Status, statusTodo)
	}
}

func TestMoveBoundaryErrors(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Boundary task") // starts at "backlog" (first)

	// --prev at first status.
	errResp := runKanbanJSONError(t, kanbanDir, "move", "1", "--prev")
	if errResp.Code != "BOUNDARY_ERROR" {
		t.Errorf("code = %q, want BOUNDARY_ERROR", errResp.Code)
	}
	if !strings.Contains(errResp.Error, "first") {
		t.Errorf("error = %q, want 'first'", errResp.Error)
	}

	// Move to last status (archived), then try --next.
	runKanban(t, kanbanDir, "--json", "move", "1", "archived")
	errResp = runKanbanJSONError(t, kanbanDir, "move", "1", "--next")
	if errResp.Code != "BOUNDARY_ERROR" {
		t.Errorf("code = %q, want BOUNDARY_ERROR", errResp.Code)
	}
	if !strings.Contains(errResp.Error, "last") {
		t.Errorf("error = %q, want 'last'", errResp.Error)
	}
}

func TestMoveNoStatusSpecified(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "No direction")

	errResp := runKanbanJSONError(t, kanbanDir, "move", "1")
	if errResp.Code != codeInvalidInput {
		t.Errorf("code = %q, want INVALID_INPUT", errResp.Code)
	}
	if !strings.Contains(errResp.Error, "provide a target status") {
		t.Errorf("error = %q, want 'provide a target status'", errResp.Error)
	}
}

func TestMoveIdempotentJSON(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Idempotent task") // starts at "backlog"

	// Move to same status should succeed with changed=false.
	var got struct {
		taskJSON
		Changed bool `json:"changed"`
	}
	r := runKanbanJSON(t, kanbanDir, &got, "move", "1", "backlog")
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", r.exitCode, r.stderr)
	}
	if got.Changed {
		t.Error("Changed = true, want false for same-status move")
	}
	if got.Status != statusBacklog {
		t.Errorf("Status = %q, want %q", got.Status, statusBacklog)
	}
}

func TestMoveIdempotentHumanOutput(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Idempotent human") // starts at "backlog"

	r := runKanban(t, kanbanDir, "--table", "move", "1", "backlog")
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stdout, "already at") {
		t.Errorf("stdout = %q, want 'already at'", r.stdout)
	}
}

func TestMoveChangedTrue(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Changed task") // starts at "backlog"

	var got struct {
		taskJSON
		Changed bool `json:"changed"`
	}
	r := runKanbanJSON(t, kanbanDir, &got, "move", "1", statusTodo)
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", r.exitCode, r.stderr)
	}
	if !got.Changed {
		t.Error("Changed = false, want true for status change")
	}
	if got.Status != statusTodo {
		t.Errorf("Status = %q, want %q", got.Status, statusTodo)
	}
}

// ---------------------------------------------------------------------------
// WIP limit tests
// ---------------------------------------------------------------------------

// initBoardWithWIP creates a board with WIP limits on in-progress.

// ---------------------------------------------------------------------------
// WIP limit tests
// ---------------------------------------------------------------------------

// initBoardWithWIP creates a board with WIP limits on in-progress.
func initBoardWithWIP(t *testing.T, limit int) string {
	t.Helper()

	dir := t.TempDir()
	initGitRepo(t, dir)
	kanbanDir := filepath.Join(dir, "kanban")

	args := []string{
		"--dir", kanbanDir, "init",
		"--wip-limit", "in-progress:" + strconv.Itoa(limit),
	}
	cmd := exec.Command(binPath, args...) //nolint:gosec,noctx // e2e test binary

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("init board with WIP: %v\nstderr: %s", err, stderr.String())
	}

	return kanbanDir
}

func TestMoveRespectsWIPLimit(t *testing.T) {
	kanbanDir := initBoardWithWIP(t, 1) // limit 1 for in-progress

	// Create a task and move it to in-progress (fills the slot).
	mustCreateTask(t, kanbanDir, "Task A")
	runKanban(t, kanbanDir, "--json", "move", "1", "in-progress", "--claim", claimTestAgent)

	// Create another task and try to move it to in-progress.
	mustCreateTask(t, kanbanDir, "Task B")
	errResp := runKanbanJSONError(t, kanbanDir, "move", "2", "in-progress", "--claim", claimTestAgent)
	if errResp.Code != codeWIPLimitExceeded {
		t.Errorf("code = %q, want WIP_LIMIT_EXCEEDED", errResp.Code)
	}
}

func TestWIPUnlimitedByDefault(t *testing.T) {
	kanbanDir := initBoard(t) // no WIP limits

	// Create many tasks in in-progress — should all succeed.
	for i := 1; i <= 5; i++ {
		mustCreateTask(t, kanbanDir, "Task "+strconv.Itoa(i), "--status", "in-progress")
	}
}

func TestInitWithWIPLimits(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	kanbanDir := filepath.Join(dir, "kanban")

	r := runKanban(t, kanbanDir, "--json", "init",
		"--wip-limit", "in-progress:3",
		"--wip-limit", "review:2")
	if r.exitCode != 0 {
		t.Fatalf("init with WIP limits failed (exit %d): %s", r.exitCode, r.stderr)
	}

	// Create a task to verify the board works.
	mustCreateTask(t, kanbanDir, "Test task")
}

// ---------------------------------------------------------------------------
// Timestamp tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Timestamp tests
// ---------------------------------------------------------------------------

func TestMoveStartedTimestamp(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Task A")

	// Move from backlog (initial) to todo — should set started.
	var task map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &task, "move", "1", statusTodo)
	if r.exitCode != 0 {
		t.Fatalf("move failed: %s", r.stderr)
	}
	if task["started"] == nil {
		t.Error("started should be set on first move from initial status")
	}
}

func TestMoveCompletedTimestamp(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Task A")
	runKanban(t, kanbanDir, "--json", "move", "1", statusTodo)

	// Move to done (terminal) — should set completed.
	var task map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &task, "move", "1", "done")
	if r.exitCode != 0 {
		t.Fatalf("move failed: %s", r.stderr)
	}
	if task["completed"] == nil {
		t.Error("completed should be set on move to terminal status")
	}
}

func TestMoveBackClearsCompleted(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Task A")
	runKanban(t, kanbanDir, "--json", "move", "1", "done")

	// Move back from done — should clear completed. Review requires --claim.
	var task map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &task, "move", "1", "review", "--claim", claimTestAgent)
	if r.exitCode != 0 {
		t.Fatalf("move failed: %s", r.stderr)
	}
	if task["completed"] != nil {
		t.Error("completed should be cleared when moving back from terminal")
	}
	if task["started"] == nil {
		t.Error("started should be preserved when moving back from terminal")
	}
}

func TestMoveStartedNeverOverwritten(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Task A")

	// First move: backlog -> todo (sets started).
	var first map[string]interface{}
	runKanbanJSON(t, kanbanDir, &first, "move", "1", statusTodo)
	started1 := first["started"]

	// Second move: todo -> in-progress (should NOT change started).
	var second map[string]interface{}
	runKanbanJSON(t, kanbanDir, &second, "move", "1", "in-progress", "--claim", claimTestAgent)
	started2 := second["started"]

	if started1 != started2 {
		t.Errorf("started changed: %v → %v", started1, started2)
	}
}

func TestMoveDirectToTerminal(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Task A")

	// Move directly from backlog to done.
	var task map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &task, "move", "1", "done")
	if r.exitCode != 0 {
		t.Fatalf("move failed: %s", r.stderr)
	}
	if task["started"] == nil {
		t.Error("started should be set on direct move to terminal")
	}
	if task["completed"] == nil {
		t.Error("completed should be set on direct move to terminal")
	}
}

func TestMoveBlockedTaskWarns(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Blocked mover")
	runKanban(t, kanbanDir, "--json", "edit", "1", "--block", "waiting")

	r := runKanban(t, kanbanDir, "--json", "move", "1", statusTodo)
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (move should succeed)", r.exitCode)
	}
	if !strings.Contains(r.stderr, "Warning") || !strings.Contains(r.stderr, "blocked") {
		t.Errorf("stderr = %q, want warning about blocked task", r.stderr)
	}
}

// ---------------------------------------------------------------------------
// Board summary tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Class-aware WIP limit tests
// ---------------------------------------------------------------------------

func TestExpediteBypassesColumnWIP(t *testing.T) {
	kanbanDir := initBoardWithWIP(t, 1)
	// Fill the in-progress column.
	mustCreateTask(t, kanbanDir, "Normal task")
	runKanban(t, kanbanDir, "move", "1", "in-progress", "--claim", claimTestAgent)

	// Create an expedite task and move it — should bypass the column WIP limit.
	mustCreateTask(t, kanbanDir, "Expedite task", "--class", "expedite")
	r := runKanban(t, kanbanDir, "move", "2", "in-progress", "--claim", claimTestAgent)
	if r.exitCode != 0 {
		t.Fatalf("expedite move failed (exit %d): %s", r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stdout, "Moved task #2") {
		t.Error("expected successful move message for expedite task")
	}
}

func TestExpediteBoardWideWIPLimit(t *testing.T) {
	kanbanDir := initBoardWithWIP(t, 1)
	// Create an expedite task (fills the board-wide limit of 1).
	mustCreateTask(t, kanbanDir, "Expedite 1", "--class", "expedite")

	// Write a second expedite task file directly to bypass create-time WIP check.
	writeTaskFile(t, kanbanDir, 2, `---
id: 2
title: Expedite 2
status: backlog
priority: medium
class: expedite
created: 2026-01-01T00:00:00Z
updated: 2026-01-01T00:00:00Z
---
`)
	bumpNextID(t, kanbanDir, 3)

	// Moving the second expedite task should be blocked by board-wide class limit.
	errResp := runKanbanJSONError(t, kanbanDir, "move", "2", statusTodo)
	if errResp.Code != "CLASS_WIP_EXCEEDED" {
		t.Errorf("error code = %q, want CLASS_WIP_EXCEEDED", errResp.Code)
	}
}

// ---------------------------------------------------------------------------
// Move command: claim during move, compact output
// ---------------------------------------------------------------------------

func TestMoveWithClaim(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Claim during move")

	var moved taskJSON
	r := runKanbanJSON(t, kanbanDir, &moved, "move", "1", "in-progress", "--claim", claimAgent1)
	if r.exitCode != 0 {
		t.Fatalf("move --claim failed (exit %d): %s", r.exitCode, r.stderr)
	}
	if moved.ClaimedBy != claimAgent1 {
		t.Errorf("claimed_by = %q, want %q", moved.ClaimedBy, claimAgent1)
	}
	if moved.Status != statusInProgress {
		t.Errorf("status = %q, want %q", moved.Status, "in-progress")
	}
}

func TestMoveCompactOutput(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Compact move test")

	r := runKanban(t, kanbanDir, "move", "1", statusTodo, "--compact")
	if r.exitCode != 0 {
		t.Fatalf("move --compact failed (exit %d): %s", r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stdout, "Moved task #1") {
		t.Error("compact move output should contain 'Moved task #1'")
	}
}

// ---------------------------------------------------------------------------
// Delete command: compact output, JSON output
// ---------------------------------------------------------------------------
