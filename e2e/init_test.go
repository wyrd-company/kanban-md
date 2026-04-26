package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Init tests
// ---------------------------------------------------------------------------

func TestInitDefault(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	kanbanDir := filepath.Join(dir, "kanban")

	var got map[string]string
	r := runKanbanJSON(t, kanbanDir, &got, "init")

	if r.exitCode != 0 {
		t.Fatalf("init failed (exit %d): %s", r.exitCode, r.stderr)
	}

	if got["status"] != "initialized" {
		t.Errorf("status = %q, want %q", got["status"], "initialized")
	}

	// Verify files on disk.
	if _, err := os.Stat(filepath.Join(kanbanDir, "config.yml")); err != nil {
		t.Errorf("config.yml not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(kanbanDir, "tasks")); !os.IsNotExist(err) {
		t.Errorf("tasks/ should not be created, stat err: %v", err)
	}
}

func TestInitWithName(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	kanbanDir := filepath.Join(dir, "kanban")

	var got map[string]string
	runKanbanJSON(t, kanbanDir, &got, "init", "--name", "My Project")

	if got["name"] != "My Project" {
		t.Errorf("name = %q, want %q", got["name"], "My Project")
	}
}

func TestInitCustomStatuses(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	kanbanDir := filepath.Join(dir, "kanban")

	var got map[string]string
	runKanbanJSON(t, kanbanDir, &got, "init", "--statuses", "open,closed")

	if got["columns"] != "open,closed" {
		t.Errorf("columns = %q, want %q", got["columns"], "open,closed")
	}
}

func TestInitAlreadyInitialized(t *testing.T) {
	kanbanDir := initBoard(t)
	r := runKanban(t, kanbanDir, "init")

	if r.exitCode == 0 {
		t.Error("expected non-zero exit code for double init")
	}
	if !strings.Contains(r.stderr, "already initialized") {
		t.Errorf("stderr = %q, want 'already initialized'", r.stderr)
	}
}

// ---------------------------------------------------------------------------
// Create tests
// ---------------------------------------------------------------------------

func TestInitShowsSkillHint(t *testing.T) {
	kanbanDir := initBoard(t)
	_ = kanbanDir
	// Re-run init to capture output (initBoard doesn't return stdout).
	dir := t.TempDir()
	initGitRepo(t, dir)
	kanbanDir2 := filepath.Join(dir, "kanban")
	r := runKanban(t, kanbanDir2, "init")
	if r.exitCode != 0 {
		t.Fatalf("init failed: %s", r.stderr)
	}
	if !strings.Contains(r.stdout, "skill install") {
		t.Errorf("init output should hint about skill install, got:\n%s", r.stdout)
	}
}

// ---------------------------------------------------------------------------
// Claim timeout / enforcement e2e tests
// ---------------------------------------------------------------------------

// writeTaskFile writes a raw task markdown file into the tasks directory.
// The filename follows the convention: 001-<slug>.md (zero-padded ID + slug).
// The title is extracted from the YAML frontmatter.
