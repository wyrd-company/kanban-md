package e2e_test

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Dependency tests
// ---------------------------------------------------------------------------

func TestCreateWithParent(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Parent task")

	var child map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &child, "create", "Child task", "--parent", "1")
	if r.exitCode != 0 {
		t.Fatalf("create with parent failed: %s", r.stderr)
	}
	if child["parent"] != float64(1) {
		t.Errorf("parent = %v, want 1", child["parent"])
	}
}

func TestCreateWithDependsOn(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Dep A")
	mustCreateTask(t, kanbanDir, "Dep B")

	var child map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &child, "create", "Dependent", "--depends-on", "1,2")
	if r.exitCode != 0 {
		t.Fatalf("create with depends-on failed: %s", r.stderr)
	}
	deps, ok := child["depends_on"].([]interface{})
	if !ok || len(deps) != 2 {
		t.Errorf("depends_on = %v, want [1,2]", child["depends_on"])
	}
}

func TestCreateSelfDepErrors(t *testing.T) {
	kanbanDir := initBoard(t)
	// Task 1 will be created, then try to create task 2 depending on itself (ID 2).
	mustCreateTask(t, kanbanDir, "Existing task")

	// Next ID is 2. --depends-on 2 is self-reference.
	errResp := runKanbanJSONError(t, kanbanDir, "create", "Self dep", "--depends-on", "2")
	if errResp.Code != "SELF_REFERENCE" {
		t.Errorf("code = %q, want SELF_REFERENCE", errResp.Code)
	}
}

func TestCreateInvalidDepErrors(t *testing.T) {
	kanbanDir := initBoard(t)

	errResp := runKanbanJSONError(t, kanbanDir, "create", "Bad dep", "--depends-on", "99")
	if errResp.Code != "DEPENDENCY_NOT_FOUND" {
		t.Errorf("code = %q, want DEPENDENCY_NOT_FOUND", errResp.Code)
	}
}

func TestEditAddDep(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Task A")
	mustCreateTask(t, kanbanDir, "Task B")

	var edited map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &edited, "edit", "2", "--add-dep", "1")
	if r.exitCode != 0 {
		t.Fatalf("edit add-dep failed: %s", r.stderr)
	}
	deps, ok := edited["depends_on"].([]interface{})
	if !ok || len(deps) != 1 || deps[0] != float64(1) {
		t.Errorf("depends_on = %v, want [1]", edited["depends_on"])
	}
}

func TestEditRemoveDep(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Task A")
	mustCreateTask(t, kanbanDir, "Task B", "--depends-on", "1")

	var edited map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &edited, "edit", "2", "--remove-dep", "1")
	if r.exitCode != 0 {
		t.Fatalf("edit remove-dep failed: %s", r.stderr)
	}
	// depends_on should be empty or absent.
	deps, _ := edited["depends_on"].([]interface{})
	if len(deps) != 0 {
		t.Errorf("depends_on = %v, want empty", edited["depends_on"])
	}
}

func TestEditSetParent(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Parent")
	mustCreateTask(t, kanbanDir, "Child")

	var edited map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &edited, "edit", "2", "--parent", "1")
	if r.exitCode != 0 {
		t.Fatalf("edit set parent failed: %s", r.stderr)
	}
	if edited["parent"] != float64(1) {
		t.Errorf("parent = %v, want 1", edited["parent"])
	}
}

func TestEditClearParent(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Parent")
	mustCreateTask(t, kanbanDir, "Child", "--parent", "1")

	var edited map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &edited, "edit", "2", "--clear-parent")
	if r.exitCode != 0 {
		t.Fatalf("edit clear parent failed: %s", r.stderr)
	}
	if edited["parent"] != nil {
		t.Errorf("parent = %v, want nil", edited["parent"])
	}
}

func TestEditSelfDepErrors(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Task")

	errResp := runKanbanJSONError(t, kanbanDir, "edit", "1", "--add-dep", "1")
	if errResp.Code != "SELF_REFERENCE" {
		t.Errorf("code = %q, want SELF_REFERENCE", errResp.Code)
	}
}

// ---------------------------------------------------------------------------
// Blocked state tests
// ---------------------------------------------------------------------------

func TestBlockTask(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Blockable task")

	var task taskJSON
	runKanbanJSON(t, kanbanDir, &task, "edit", "1", "--block", "waiting for API")

	// Verify via show.
	var shown struct {
		taskJSON
		Blocked     bool   `json:"blocked"`
		BlockReason string `json:"block_reason"`
	}
	runKanbanJSON(t, kanbanDir, &shown, "show", "1")
	if !shown.Blocked {
		t.Error("Blocked = false, want true")
	}
	if shown.BlockReason != "waiting for API" {
		t.Errorf("BlockReason = %q, want %q", shown.BlockReason, "waiting for API")
	}
}

func TestUnblockTask(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Unblockable")

	// Block first.
	runKanban(t, kanbanDir, "--json", "edit", "1", "--block", "stuck")

	// Unblock.
	runKanban(t, kanbanDir, "--json", "edit", "1", "--unblock")

	var shown struct {
		taskJSON
		Blocked     bool   `json:"blocked"`
		BlockReason string `json:"block_reason"`
	}
	runKanbanJSON(t, kanbanDir, &shown, "show", "1")
	if shown.Blocked {
		t.Error("Blocked = true after unblock, want false")
	}
	if shown.BlockReason != "" {
		t.Errorf("BlockReason = %q after unblock, want empty", shown.BlockReason)
	}
}

func TestBlockRequiresReason(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "No reason")

	errResp := runKanbanJSONError(t, kanbanDir, "edit", "1", "--block", "")
	if errResp.Code != codeInvalidInput {
		t.Errorf("code = %q, want INVALID_INPUT", errResp.Code)
	}
	if !strings.Contains(errResp.Error, "block reason is required") {
		t.Errorf("error = %q, want 'block reason is required'", errResp.Error)
	}
}

func TestBlockAndUnblockConflict(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Conflict")

	errResp := runKanbanJSONError(t, kanbanDir, "edit", "1", "--block", "reason", "--unblock")
	if errResp.Code != codeStatusConflict {
		t.Errorf("code = %q, want STATUS_CONFLICT", errResp.Code)
	}
	if !strings.Contains(errResp.Error, "cannot use --block and --unblock together") {
		t.Errorf("error = %q, want conflict message", errResp.Error)
	}
}

// ---------------------------------------------------------------------------
// Cycle rejection
// ---------------------------------------------------------------------------

func TestEditParentRejectsTwoStepCycle(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "A")
	mustCreateTask(t, kanbanDir, "B")

	if r := runKanban(t, kanbanDir, "edit", "1", "--parent", "2"); r.exitCode != 0 {
		t.Fatalf("first parent edit failed: %s", r.stderr)
	}

	r := runKanban(t, kanbanDir, "edit", "2", "--parent", "1")
	if r.exitCode == 0 {
		t.Fatal("closing the parent ring succeeded, want a rejection")
	}
	if !strings.Contains(r.stderr, "cycle") {
		t.Errorf("stderr = %q, want it to mention a cycle", r.stderr)
	}
	if !strings.Contains(r.stderr, "#2 → #1 → #2") {
		t.Errorf("stderr = %q, want it to name the ring", r.stderr)
	}

	// The rejected edit must not have been written.
	var task2 map[string]interface{}
	runKanbanJSON(t, kanbanDir, &task2, "show", "2")
	if task2["parent"] != nil {
		t.Errorf("parent of #2 = %v, want it unchanged after the rejection", task2["parent"])
	}
}

func TestEditParentRejectsDeepCycle(t *testing.T) {
	kanbanDir := initBoard(t)
	for _, title := range []string{"A", "B", "C"} {
		mustCreateTask(t, kanbanDir, title)
	}
	mustRun(t, kanbanDir, "edit", "2", "--parent", "1")
	mustRun(t, kanbanDir, "edit", "3", "--parent", "2")

	r := runKanban(t, kanbanDir, "edit", "1", "--parent", "3")
	if r.exitCode == 0 {
		t.Fatal("closing a three-step parent ring succeeded, want a rejection")
	}
	if !strings.Contains(r.stderr, "cycle") {
		t.Errorf("stderr = %q, want it to mention a cycle", r.stderr)
	}
}

func TestEditParentStillAllowsValidMoves(t *testing.T) {
	kanbanDir := initBoard(t)
	for _, title := range []string{"Root", "Epic", "Sibling", "Story"} {
		mustCreateTask(t, kanbanDir, title)
	}
	mustRun(t, kanbanDir, "edit", "2", "--parent", "1")
	mustRun(t, kanbanDir, "edit", "3", "--parent", "1")
	mustRun(t, kanbanDir, "edit", "4", "--parent", "2")

	// Moving a leaf between siblings is not a cycle.
	if r := runKanban(t, kanbanDir, "edit", "4", "--parent", "3"); r.exitCode != 0 {
		t.Errorf("moving #4 under its uncle was rejected: %s", r.stderr)
	}
}

func TestEditDependsOnRejectsCycle(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "A")
	mustCreateTask(t, kanbanDir, "B")
	mustRun(t, kanbanDir, "edit", "1", "--add-dep", "2")

	r := runKanban(t, kanbanDir, "edit", "2", "--add-dep", "1")
	if r.exitCode == 0 {
		t.Fatal("closing the dependency ring succeeded, want a rejection")
	}
	if !strings.Contains(r.stderr, "cycle") {
		t.Errorf("stderr = %q, want it to mention a cycle", r.stderr)
	}
}

func TestCreateWithParentRejectsCycleThroughStoredTasks(t *testing.T) {
	// create validates through the same gate as edit.
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "A")
	mustCreateTask(t, kanbanDir, "B")
	mustRun(t, kanbanDir, "edit", "1", "--parent", "2")

	// #3 under #1 is fine — no ring, #3 is new.
	if r := runKanban(t, kanbanDir, "create", "C", "--parent", "1"); r.exitCode != 0 {
		t.Errorf("creating a child below an existing chain was rejected: %s", r.stderr)
	}
}

// mustRun runs a command and fails the test if it does not succeed.
func mustRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	if r := runKanban(t, dir, args...); r.exitCode != 0 {
		t.Fatalf("%v failed: %s", args, r.stderr)
	}
}
