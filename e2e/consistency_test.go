package e2e_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestConsistencyRepair_DuplicateIDsAndNextID(t *testing.T) {
	kanbanDir := initBoard(t)

	mustCreateTask(t, kanbanDir, "First task")
	mustCreateTask(t, kanbanDir, "Second task")

	// Corrupt task #2 by duplicating frontmatter ID with task #1.
	task2Path := filepath.Join(kanbanDir, "tasks", "002-second-task.md")
	data, err := os.ReadFile(task2Path) //nolint:gosec // e2e test file
	if err != nil {
		t.Fatalf("reading task #2 file: %v", err)
	}
	corrupted := strings.Replace(string(data), "id: 2", "id: 1", 1)
	if err := os.WriteFile(task2Path, []byte(corrupted), 0o600); err != nil { //nolint:gosec // e2e test file
		t.Fatalf("writing corrupted task file: %v", err)
	}

	r := runKanban(t, kanbanDir, "list")
	if r.exitCode != 0 {
		t.Fatalf("list failed after corruption (exit %d): %s", r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stderr, "auto-repaired") {
		t.Errorf("expected auto-repair warning in stderr, got: %s", r.stderr)
	}

	var tasks []taskJSON
	r = runKanbanJSON(t, kanbanDir, &tasks, "list")
	if r.exitCode != 0 {
		t.Fatalf("list --json failed (exit %d): %s", r.exitCode, r.stderr)
	}
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks))
	}
	ids := map[int]bool{}
	for _, tk := range tasks {
		if ids[tk.ID] {
			t.Fatalf("duplicate ID remained after repair: %d", tk.ID)
		}
		ids[tk.ID] = true
	}
	if !ids[1] || !ids[3] {
		t.Fatalf("repaired IDs = %v, want IDs 1 and 3", ids)
	}

	var created taskJSON
	r = runKanbanJSON(t, kanbanDir, &created, "create", "Third task")
	if r.exitCode != 0 {
		t.Fatalf("create after repair failed (exit %d): %s", r.exitCode, r.stderr)
	}
	if created.ID != 4 {
		t.Errorf("created ID = %d, want 4 after next_id repair", created.ID)
	}
}

func TestConsistencyRepair_FilenameFrontmatterMismatch(t *testing.T) {
	kanbanDir := initBoard(t)
	mustCreateTask(t, kanbanDir, "Mismatch task")

	oldPath := filepath.Join(kanbanDir, "tasks", "001-mismatch-task.md")
	newPath := filepath.Join(kanbanDir, "tasks", "099-mismatch-task.md")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("renaming task file to mismatch ID: %v", err)
	}

	r := runKanban(t, kanbanDir, "show", "1")
	if r.exitCode != 0 {
		t.Fatalf("show should succeed after auto-repair (exit %d): %s", r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stdout, "Task #1: Mismatch task") {
		t.Errorf("show output missing repaired task details, got:\n%s", r.stdout)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("expected repaired filename %s to exist: %v", oldPath, err)
	}
}

func TestConsistencyRepair_NextIDDrift(t *testing.T) {
	kanbanDir := initBoard(t)

	writeTaskFile(t, kanbanDir, 10, `---
id: 10
title: Manually added task
status: backlog
priority: medium
created: 2026-02-24T12:00:00Z
updated: 2026-02-24T12:00:00Z
---
`)
	// Intentionally keep next_id low to simulate drift.
	bumpNextID(t, kanbanDir, 1)

	var created taskJSON
	r := runKanbanJSON(t, kanbanDir, &created, "create", "Created after drift")
	if r.exitCode != 0 {
		t.Fatalf("create failed (exit %d): %s", r.exitCode, r.stderr)
	}
	if created.ID != 11 {
		t.Errorf("created ID = %d, want 11 (max existing ID + 1)", created.ID)
	}

	// Verify config was advanced, not just this one create result.
	cfgData, err := os.ReadFile(filepath.Join(kanbanDir, "config.yml")) //nolint:gosec // e2e test file
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if !strings.Contains(string(cfgData), "next_id: "+strconv.Itoa(created.ID+1)) {
		t.Errorf("config next_id not advanced after repair, config:\n%s", string(cfgData))
	}
}
