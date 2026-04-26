package store

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/antopolskiy/kanban-md/internal/config"
	"github.com/antopolskiy/kanban-md/internal/task"
)

func TestGitStoreInitializeMutateAndLoad(t *testing.T) {
	cfg := newStoreTestConfig(t)
	ctx := context.Background()
	st, err := NewGitStore(ctx, cfg)
	if err != nil {
		t.Fatalf("NewGitStore() error: %v", err)
	}
	if _, initErr := st.Initialize(ctx); initErr != nil {
		t.Fatalf("Initialize() error: %v", initErr)
	}

	now := time.Now()
	_, err = st.Mutate(ctx, func(snap *Snapshot) error {
		snap.Tasks = append(snap.Tasks, &task.Task{
			ID:       snap.NextID,
			Title:    "Write snapshot test",
			Status:   "backlog",
			Priority: "medium",
			Created:  now,
			Updated:  now,
			File:     "tasks/001-write-snapshot-test.md",
		})
		snap.NextID++
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate() error: %v", err)
	}

	snap, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if snap.NextID != 2 {
		t.Fatalf("NextID = %d, want 2", snap.NextID)
	}
	if len(snap.Tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(snap.Tasks))
	}
	if snap.Tasks[0].Title != "Write snapshot test" {
		t.Fatalf("task title = %q", snap.Tasks[0].Title)
	}
}

func TestGitStoreLoadRejectsMalformedSnapshot(t *testing.T) {
	ctx := context.Background()
	cfg := newStoreTestConfig(t)
	st, err := NewGitStore(ctx, cfg)
	if err != nil {
		t.Fatalf("NewGitStore() error: %v", err)
	}
	initSnap, err := st.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	now := time.Now()
	_, err = st.commitSnapshot(ctx, &Snapshot{
		NextID: 1,
		Tasks: []*task.Task{{
			ID:       10,
			Title:    "Manual drift",
			Status:   "backlog",
			Priority: "medium",
			Created:  now,
			Updated:  now,
			File:     "tasks/010-manual-drift.md",
		}},
	}, initSnap.Rev)
	if err != nil {
		t.Fatalf("commitSnapshot() error: %v", err)
	}

	_, err = st.Load(ctx)
	if err == nil || !strings.Contains(err.Error(), "meta.next_id") {
		t.Fatalf("Load() error = %v, want next_id drift error", err)
	}
}

func newStoreTestConfig(t *testing.T) *config.Config {
	t.Helper()
	repo := t.TempDir()
	cmd := exec.CommandContext(context.Background(), "git", "-C", repo, "init") //nolint:gosec // test fixture path from t.TempDir.
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	cfg := config.NewDefault("Test")
	cfg.SetDir(repo)
	cfg.TasksDir = ""
	cfg.NextID = 0
	return cfg
}
