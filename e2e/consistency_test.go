package e2e_test

import (
	"strings"
	"testing"
)

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

	r := runKanban(t, kanbanDir, "create", "Created after drift")
	if r.exitCode == 0 {
		t.Fatalf("create unexpectedly repaired malformed snapshot: %s", r.stdout)
	}
	if !strings.Contains(r.stderr, "meta.next_id") {
		t.Fatalf("stderr = %q, want strict next_id drift error", r.stderr)
	}
}
