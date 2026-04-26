package e2e_test

import (
	"strings"
	"testing"
)

func TestDeleteWithDependentsWarns(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Dep task")                          // #1
	mustCreateTask(t, kanbanDir, "Depends on 1", "--depends-on", "1") // #2

	r := runKanban(t, kanbanDir, "--json", "delete", "1", "--yes")
	if r.exitCode != 0 {
		t.Fatalf("delete failed: %s", r.stderr)
	}
	if !strings.Contains(r.stderr, "depends on this task") {
		t.Errorf("stderr = %q, want dependent warning", r.stderr)
	}

	// Dependent should remain recoverable via --unblocked after delete.
	var tasks []taskJSON
	runKanbanJSON(t, kanbanDir, &tasks, "list", "--unblocked")
	if len(tasks) != 1 || tasks[0].ID != 2 {
		t.Fatalf("unblocked tasks after delete = %+v, want only task #2", tasks)
	}
}

// ---------------------------------------------------------------------------
// Delete tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Delete tests
// ---------------------------------------------------------------------------

func TestDeleteWithYes(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Doomed task")

	var got map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &got, "delete", "1", "--yes")

	if r.exitCode != 0 {
		t.Fatalf("delete failed: %s", r.stderr)
	}
	if got["status"] != statusDeleted {
		t.Errorf("status = %v, want %q", got["status"], statusDeleted)
	}

	var shown taskJSON
	runKanbanJSON(t, kanbanDir, &shown, "show", "1")
	if shown.Status != statusArchived {
		t.Errorf("status = %q, want %q", shown.Status, statusArchived)
	}
}

func TestDeleteWithoutYesNonTTY(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Protected task")

	errResp := runKanbanJSONError(t, kanbanDir, "delete", "1")
	if errResp.Code != "CONFIRMATION_REQUIRED" {
		t.Errorf("code = %q, want CONFIRMATION_REQUIRED", errResp.Code)
	}
	if !strings.Contains(errResp.Error, "not a terminal") {
		t.Errorf("error = %q, want 'not a terminal'", errResp.Error)
	}
}

// ---------------------------------------------------------------------------
// Cross-cutting tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Delete command: compact output, JSON output
// ---------------------------------------------------------------------------

func TestDeleteJSONOutput(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Delete JSON test")

	var result map[string]interface{}
	r := runKanbanJSON(t, kanbanDir, &result, "delete", "1", "--yes")
	if r.exitCode != 0 {
		t.Fatalf("delete --json failed (exit %d): %s", r.exitCode, r.stderr)
	}
	if status, ok := result["status"].(string); !ok || status != statusDeleted {
		t.Errorf("JSON status = %v, want %q", result["status"], statusDeleted)
	}
}

func TestDeleteTableOutput(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Delete table test")

	r := runKanban(t, kanbanDir, "delete", "1", "--yes")
	if r.exitCode != 0 {
		t.Fatalf("delete --yes failed (exit %d): %s", r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stdout, "Deleted task #1") {
		t.Error("table output should contain 'Deleted task #1'")
	}
}

// ---------------------------------------------------------------------------
// Archive command tests
// ---------------------------------------------------------------------------
