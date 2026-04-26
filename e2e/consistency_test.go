package e2e_test

import "testing"

func TestRefSnapshotNextIDDrift(t *testing.T) {
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
	bumpNextID(t, kanbanDir, 1)

	var created taskJSON
	r := runKanbanJSON(t, kanbanDir, &created, "create", "Created after drift")
	if r.exitCode != 0 {
		t.Fatalf("create failed (exit %d): %s", r.exitCode, r.stderr)
	}
	if created.ID != 11 {
		t.Errorf("created ID = %d, want 11 (max existing ID + 1)", created.ID)
	}
}
