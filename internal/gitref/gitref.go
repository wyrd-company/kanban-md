// Package gitref contains small Git plumbing helpers for ref-backed board state.
package gitref

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrUpdateRef indicates that a compare-and-swap ref update failed.
var ErrUpdateRef = errors.New("updating git ref")

// Repository is a Git repository addressed by its work tree.
type Repository struct {
	WorkDir string
}

// Open locates the Git work tree containing startDir.
func Open(ctx context.Context, startDir string) (*Repository, error) {
	out, err := run(ctx, startDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("locating git repository: %w", err)
	}
	return &Repository{WorkDir: strings.TrimSpace(string(out))}, nil
}

// ResolveRef resolves ref to a commit OID. The bool is false when the ref is missing.
func (r *Repository) ResolveRef(ctx context.Context, ref string) (string, bool, error) {
	out, err := r.run(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		if len(out) == 0 {
			return "", false, nil
		}
		return "", false, fmt.Errorf("resolving %s: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), true, nil
}

// ListFiles lists all paths in a commit tree.
func (r *Repository) ListFiles(ctx context.Context, rev string) ([]string, error) {
	out, err := r.run(ctx, "ls-tree", "-r", "--name-only", rev)
	if err != nil {
		return nil, fmt.Errorf("listing %s tree: %w", rev, err)
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

// ReadFile reads path from rev.
func (r *Repository) ReadFile(ctx context.Context, rev, path string) ([]byte, error) {
	out, err := r.run(ctx, "show", rev+":"+path)
	if err != nil {
		return nil, fmt.Errorf("reading %s:%s: %w", rev, path, err)
	}
	return out, nil
}

// TreeEntry describes one git tree entry.
type TreeEntry struct {
	Mode string
	Type string
	OID  string
	Path string
}

// WriteBlob writes data as a blob and returns its OID.
func (r *Repository) WriteBlob(ctx context.Context, data []byte) (string, error) {
	out, err := r.runStdin(ctx, data, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", fmt.Errorf("writing blob: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// MakeTree creates a tree from entries and returns its OID.
func (r *Repository) MakeTree(ctx context.Context, entries []TreeEntry) (string, error) {
	var input bytes.Buffer
	for _, entry := range entries {
		fmt.Fprintf(&input, "%s %s %s\t%s\n", entry.Mode, entry.Type, entry.OID, entry.Path)
	}
	out, err := r.runStdin(ctx, input.Bytes(), "mktree")
	if err != nil {
		return "", fmt.Errorf("creating tree: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CommitTree creates a commit for tree and returns its OID.
func (r *Repository) CommitTree(ctx context.Context, treeOID, parentOID, message string) (string, error) {
	args := []string{"commit-tree", treeOID}
	if parentOID != "" {
		args = append(args, "-p", parentOID)
	}
	args = append(args, "-m", message)
	out, err := r.run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("creating commit: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// UpdateRef updates ref to newOID, optionally requiring oldOID.
func (r *Repository) UpdateRef(ctx context.Context, ref, newOID, oldOID string) error {
	args := []string{"update-ref", ref, newOID}
	if oldOID != "" {
		args = append(args, oldOID)
	}
	if _, err := r.run(ctx, args...); err != nil {
		return fmt.Errorf("%w %s: %w", ErrUpdateRef, ref, err)
	}
	return nil
}

func (r *Repository) run(ctx context.Context, args ...string) ([]byte, error) {
	return run(ctx, r.WorkDir, args...)
}

func (r *Repository) runStdin(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	return runStdin(ctx, r.WorkDir, input, args...)
}

func run(ctx context.Context, workDir string, args ...string) ([]byte, error) {
	return runStdin(ctx, workDir, nil, args...)
}

func runStdin(ctx context.Context, workDir string, input []byte, args ...string) ([]byte, error) {
	cmdArgs := append([]string{"-C", workDir}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...) //nolint:gosec // git args are fixed plumbing commands plus trusted repo paths.
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=kanban-md",
		"GIT_AUTHOR_EMAIL=kanban-md@example.invalid",
		"GIT_COMMITTER_NAME=kanban-md",
		"GIT_COMMITTER_EMAIL=kanban-md@example.invalid",
	)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, errors.New(msg)
	}
	return out, nil
}
