package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/antopolskiy/kanban-md/internal/config"
)

// --- parseWIPLimits tests ---

func TestParseWIPLimits_Valid(t *testing.T) {
	limits, err := parseWIPLimits([]string{"in-progress:3", "review:5"})
	if err != nil {
		t.Fatal(err)
	}
	if limits["in-progress"] != 3 {
		t.Errorf("in-progress = %d, want 3", limits["in-progress"])
	}
	if limits["review"] != 5 {
		t.Errorf("review = %d, want 5", limits["review"])
	}
}

func TestParseWIPLimits_InvalidFormat(t *testing.T) {
	_, err := parseWIPLimits([]string{"no-colon"})
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestParseWIPLimits_InvalidNumber(t *testing.T) {
	_, err := parseWIPLimits([]string{"in-progress:abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric value")
	}
}

func TestParseWIPLimits_Empty(t *testing.T) {
	limits, err := parseWIPLimits([]string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(limits) != 0 {
		t.Errorf("expected empty map, got %v", limits)
	}
}

// --- runInit tests ---

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("name", "", "")
	cmd.Flags().StringSlice("statuses", nil, "")
	cmd.Flags().StringSlice("wip-limit", nil, "")
	return cmd
}

func initGitRepoForInitTest(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "init") //nolint:gosec // test fixture path from t.TempDir.
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
}

func TestRunInit_DefaultName(t *testing.T) {
	dir := t.TempDir()
	initGitRepoForInitTest(t, dir)
	kanbanDir := filepath.Join(dir, "kanban")

	oldFlagDir := flagDir
	flagDir = kanbanDir
	t.Cleanup(func() { flagDir = oldFlagDir })

	setFlags(t, false, true, false)
	r, w := captureStdout(t)

	cmd := newInitCmd()
	err := runInit(cmd, nil)

	got := drainPipe(t, r, w)

	if err != nil {
		t.Fatalf("runInit error: %v", err)
	}
	if !containsSubstring(got, "Initialized board") {
		t.Errorf("expected init message, got: %s", got)
	}

	// Verify config file exists.
	if _, err := os.Stat(filepath.Join(kanbanDir, config.ConfigFileName)); err != nil {
		t.Errorf("config file should exist: %v", err)
	}

	// Verify tasks directory is not created for ref-backed boards.
	if _, err := os.Stat(filepath.Join(kanbanDir, "tasks")); err == nil {
		t.Errorf("tasks directory should not exist for ref-backed boards")
	}
}

func TestRunInit_WithName(t *testing.T) {
	dir := t.TempDir()
	initGitRepoForInitTest(t, dir)
	kanbanDir := filepath.Join(dir, "kanban")

	oldFlagDir := flagDir
	flagDir = kanbanDir
	t.Cleanup(func() { flagDir = oldFlagDir })

	setFlags(t, false, true, false)
	r, w := captureStdout(t)

	cmd := newInitCmd()
	_ = cmd.Flags().Set("name", "MyProject")
	err := runInit(cmd, nil)

	_ = drainPipe(t, r, w)

	if err != nil {
		t.Fatalf("runInit error: %v", err)
	}

	cfg, err := config.Load(kanbanDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Board.Name != "MyProject" {
		t.Errorf("board name = %q, want %q", cfg.Board.Name, "MyProject")
	}
}

func TestRunInit_CustomStatuses(t *testing.T) {
	dir := t.TempDir()
	initGitRepoForInitTest(t, dir)
	kanbanDir := filepath.Join(dir, "kanban")

	oldFlagDir := flagDir
	flagDir = kanbanDir
	t.Cleanup(func() { flagDir = oldFlagDir })

	setFlags(t, false, true, false)
	r, w := captureStdout(t)

	cmd := newInitCmd()
	_ = cmd.Flags().Set("statuses", "open,closed")
	err := runInit(cmd, nil)

	_ = drainPipe(t, r, w)

	if err != nil {
		t.Fatalf("runInit error: %v", err)
	}

	cfg, err := config.Load(kanbanDir)
	if err != nil {
		t.Fatal(err)
	}
	names := cfg.StatusNames()
	if len(names) != 2 || names[0] != "open" || names[1] != "closed" {
		t.Errorf("statuses = %v, want [open, closed]", names)
	}
	if cfg.Defaults.Status != "open" {
		t.Errorf("default status = %q, want %q", cfg.Defaults.Status, "open")
	}
}

func TestRunInit_WithWIPLimits(t *testing.T) {
	dir := t.TempDir()
	initGitRepoForInitTest(t, dir)
	kanbanDir := filepath.Join(dir, "kanban")

	oldFlagDir := flagDir
	flagDir = kanbanDir
	t.Cleanup(func() { flagDir = oldFlagDir })

	setFlags(t, false, true, false)
	r, w := captureStdout(t)

	cmd := newInitCmd()
	_ = cmd.Flags().Set("wip-limit", "in-progress:3")
	err := runInit(cmd, nil)

	_ = drainPipe(t, r, w)

	if err != nil {
		t.Fatalf("runInit error: %v", err)
	}

	cfg, err := config.Load(kanbanDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WIPLimits["in-progress"] != 3 {
		t.Errorf("wip_limits[in-progress] = %d, want 3", cfg.WIPLimits["in-progress"])
	}
}

func TestRunInit_AlreadyInitialized(t *testing.T) {
	kanbanDir := setupBoard(t)

	oldFlagDir := flagDir
	flagDir = kanbanDir
	t.Cleanup(func() { flagDir = oldFlagDir })

	cmd := newInitCmd()
	err := runInit(cmd, nil)
	if err == nil {
		t.Fatal("expected error for already initialized board")
	}
}

func TestRunInit_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	initGitRepoForInitTest(t, dir)
	kanbanDir := filepath.Join(dir, "kanban")

	oldFlagDir := flagDir
	flagDir = kanbanDir
	t.Cleanup(func() { flagDir = oldFlagDir })

	setFlags(t, true, false, false)
	r, w := captureStdout(t)

	cmd := newInitCmd()
	_ = cmd.Flags().Set("name", "TestJSON")
	err := runInit(cmd, nil)

	got := drainPipe(t, r, w)

	if err != nil {
		t.Fatalf("runInit error: %v", err)
	}
	if !containsSubstring(got, `"status": "initialized"`) {
		t.Errorf("expected JSON with status:initialized, got: %s", got)
	}
}

func TestRunInit_AppendsToExistingGitignore(t *testing.T) {
	dir := t.TempDir()
	initGitRepoForInitTest(t, dir)
	kanbanDir := filepath.Join(dir, "kanban")
	gitignorePath := filepath.Join(dir, ".gitignore")
	oldContent := "tmp/\n"
	if err := os.WriteFile(gitignorePath, []byte(oldContent), 0o600); err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("\n"))
	_ = w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	oldFlagDir := flagDir
	flagDir = kanbanDir
	t.Cleanup(func() { flagDir = oldFlagDir })

	setFlags(t, false, true, false)
	rOut, wOut := captureStdout(t)

	cmd := newInitCmd()
	err = runInit(cmd, nil)
	_ = drainPipe(t, rOut, wOut)

	if err != nil {
		t.Fatalf("runInit error: %v", err)
	}

	// #nosec G304 -- this test uses a fixture path created from t.TempDir.
	got, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !containsSubstring(string(got), oldContent) {
		t.Fatalf("expected .gitignore to keep existing entries, got: %s", string(got))
	}
	if !containsSubstring(string(got), "kanban/\n") {
		t.Fatalf("expected kanban/ to be added to .gitignore, got: %s", string(got))
	}
}

func TestRunInit_SkipsGitignoreIfNo(t *testing.T) {
	dir := t.TempDir()
	initGitRepoForInitTest(t, dir)
	kanbanDir := filepath.Join(dir, "kanban")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("n\n"))
	_ = w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	oldFlagDir := flagDir
	flagDir = kanbanDir
	t.Cleanup(func() { flagDir = oldFlagDir })

	setFlags(t, false, true, false)
	rOut, wOut := captureStdout(t)

	cmd := newInitCmd()
	err = runInit(cmd, nil)
	_ = drainPipe(t, rOut, wOut)

	if err != nil {
		t.Fatalf("runInit error: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(dir, ".gitignore")); statErr == nil {
		t.Fatal("expected no .gitignore update when user declines")
	}
}
