package e2e_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// binPath holds the path to the compiled kanban-md binary.
var binPath string

// Constants used in multiple tests.
const (
	codeWIPLimitExceeded = "WIP_LIMIT_EXCEEDED"
	codeClaimRequired    = "CLAIM_REQUIRED"
	codeInvalidInput     = "INVALID_INPUT"
	codeInvalidDate      = "INVALID_DATE"
	codeInvalidStatus    = "INVALID_STATUS"
	statusBacklog        = "backlog"
	statusInProgress     = "in-progress"
	statusDeleted        = "deleted"
	statusArchived       = "archived"
	priorityHigh         = "high"
	claimTestAgent       = "test-agent"
	claimAgent1          = "agent-1"
	codeTaskClaimed      = "TASK_CLAIMED"
	codeStatusConflict   = "STATUS_CONFLICT"
	statusReview         = "review"
	statusTodo           = "todo"
	assigneeAlice        = "alice"
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "kanban-md-e2e-*")
	if err != nil {
		panic("creating temp dir: " + err.Error())
	}

	binName := "kanban-md"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath = filepath.Join(tmp, binName)

	// Build with -cover when GOCOVERDIR is requested. The coverage-instrumented
	// binary writes raw coverage data to the directory specified by GOCOVERDIR.
	buildArgs := []string{"build", "-o", binPath}
	coverDir := os.Getenv("GOCOVERDIR")
	if coverDir != "" {
		buildArgs = append(buildArgs, "-cover",
			"-coverpkg=github.com/antopolskiy/kanban-md/...")
	}
	buildArgs = append(buildArgs, "../cmd/kanban-md")

	//nolint:gosec,noctx // building test binary in TestMain (no context available)
	build := exec.Command("go", buildArgs...)
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("building binary: " + err.Error())
	}

	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// result captures command execution output.
type result struct {
	stdout   string
	stderr   string
	exitCode int
}

// taskJSON mirrors the task JSON output schema.
type taskJSON struct {
	ID          int      `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Priority    string   `json:"priority"`
	Assignee    string   `json:"assignee,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Due         string   `json:"due,omitempty"`
	Estimate    string   `json:"estimate,omitempty"`
	Body        string   `json:"body,omitempty"`
	File        string   `json:"file,omitempty"`
	Created     string   `json:"created"`
	Updated     string   `json:"updated"`
	ClaimedBy   string   `json:"claimed_by,omitempty"`
	Blocked     bool     `json:"blocked,omitempty"`
	BlockReason string   `json:"block_reason,omitempty"`
}

// runKanban executes the binary with --dir prepended for test isolation.

// runKanban executes the binary with --dir prepended for test isolation.
func runKanban(t *testing.T, dir string, args ...string) result {
	t.Helper()

	fullArgs := append([]string{"--dir", dir}, args...)
	cmd := exec.Command(binPath, fullArgs...) //nolint:gosec,noctx // e2e test binary

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	r := result{
		stdout: stdout.String(),
		stderr: stderr.String(),
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			r.exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("running kanban-md: %v", err)
		}
	}

	return r
}

// runKanbanEnv runs the kanban-md binary with extra environment variables.

// runKanbanEnv runs the kanban-md binary with extra environment variables.
func runKanbanEnv(t *testing.T, dir string, env []string, args ...string) result {
	t.Helper()

	fullArgs := append([]string{"--dir", dir}, args...)
	cmd := exec.Command(binPath, fullArgs...) //nolint:gosec,noctx // e2e test binary
	cmd.Env = append(os.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	r := result{
		stdout: stdout.String(),
		stderr: stderr.String(),
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			r.exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("running kanban-md: %v", err)
		}
	}

	return r
}

// runKanbanJSON runs with --json and unmarshals stdout into dest.

// runKanbanJSON runs with --json and unmarshals stdout into dest.
func runKanbanJSON(t *testing.T, dir string, dest interface{}, args ...string) result {
	t.Helper()

	jsonArgs := append([]string{"--json"}, args...)
	r := runKanban(t, dir, jsonArgs...)

	if r.exitCode != 0 {
		return r
	}

	if err := json.Unmarshal([]byte(r.stdout), dest); err != nil {
		t.Fatalf("parsing JSON output: %v\nstdout: %s", err, r.stdout)
	}

	return r
}

// errorJSON captures the structured error JSON output.
type errorJSON struct {
	Error   string         `json:"error"`
	Code    string         `json:"code"`
	Details map[string]any `json:"details,omitempty"`
}

// runKanbanJSONError runs with --json and expects a non-zero exit code.
// It parses the structured error from stdout.

// runKanbanJSONError runs with --json and expects a non-zero exit code.
// It parses the structured error from stdout.
func runKanbanJSONError(t *testing.T, dir string, args ...string) errorJSON {
	t.Helper()

	jsonArgs := append([]string{"--json"}, args...)
	r := runKanban(t, dir, jsonArgs...)

	if r.exitCode == 0 {
		t.Fatalf("expected non-zero exit code, got 0\nstdout: %s", r.stdout)
	}

	var errResp errorJSON
	if err := json.Unmarshal([]byte(r.stdout), &errResp); err != nil {
		t.Fatalf("parsing error JSON: %v\nstdout: %s", err, r.stdout)
	}

	return errResp
}

// initBoard initializes a board in a fresh temp directory, returns kanban dir path.

// initBoard initializes a board in a fresh temp directory, returns kanban dir path.
func initBoard(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	initGitRepo(t, dir)
	kanbanDir := filepath.Join(dir, "kanban")

	cmd := exec.Command(binPath, "--dir", kanbanDir, "init") //nolint:gosec,noctx // e2e test binary

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("init board: %v\nstderr: %s", err, stderr.String())
	}

	return kanbanDir
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "init") //nolint:gosec,noctx // e2e test helper
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v\nstderr: %s", err, stderr.String())
	}
}

// mustCreateTask creates a task and returns its parsed JSON.

// mustCreateTask creates a task and returns its parsed JSON.
func mustCreateTask(t *testing.T, dir, title string, extraArgs ...string) taskJSON {
	t.Helper()

	args := append([]string{"create", title}, extraArgs...)
	var task taskJSON
	r := runKanbanJSON(t, dir, &task, args...)
	if r.exitCode != 0 {
		t.Fatalf("create task %q failed (exit %d): %s", title, r.exitCode, r.stderr)
	}

	return task
}

// ---------------------------------------------------------------------------
// Init tests
// ---------------------------------------------------------------------------
