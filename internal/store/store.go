// Package store provides snapshot storage for kanban boards.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/antopolskiy/kanban-md/internal/config"
	"github.com/antopolskiy/kanban-md/internal/gitref"
	"github.com/antopolskiy/kanban-md/internal/task"
)

const snapshotVersion = 1

// ErrRefNotFound indicates that the configured board ref does not exist.
var ErrRefNotFound = errors.New("board storage ref not found")

// Snapshot is an in-memory view of a ref-backed board.
type Snapshot struct {
	Tasks  []*task.Task
	NextID int
	Rev    string
}

// GitStore stores a board snapshot in a single Git ref.
type GitStore struct {
	repo *gitref.Repository
	ref  string
}

type meta struct {
	Version int `json:"version"`
	NextID  int `json:"next_id"`
}

// NewGitStore creates a Git-ref-backed store from config.
func NewGitStore(ctx context.Context, cfg *config.Config) (*GitStore, error) {
	repo, err := gitref.Open(ctx, cfg.Dir())
	if err != nil {
		return nil, err
	}
	ref := cfg.Storage.Ref
	if ref == "" {
		ref = config.DefaultStorageRef
	}
	return &GitStore{repo: repo, ref: ref}, nil
}

// Ref returns the Git ref used by the store.
func (s *GitStore) Ref() string {
	return s.ref
}

// RepositoryPath returns the Git work tree path used for plumbing commands.
func (s *GitStore) RepositoryPath() string {
	return s.repo.WorkDir
}

// Load reads the current board snapshot.
func (s *GitStore) Load(ctx context.Context) (*Snapshot, error) {
	rev, ok, err := s.repo.ResolveRef(ctx, s.ref)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRefNotFound, s.ref)
	}

	files, err := s.repo.ListFiles(ctx, rev)
	if err != nil {
		return nil, err
	}

	snap := &Snapshot{Rev: rev}
	for _, name := range files {
		if name == "meta.json" {
			if err := s.readMeta(ctx, rev, snap); err != nil {
				return nil, err
			}
			continue
		}
		if strings.HasPrefix(name, "tasks/") && strings.HasSuffix(name, ".md") {
			t, err := s.readTask(ctx, rev, name)
			if err != nil {
				return nil, err
			}
			snap.Tasks = append(snap.Tasks, t)
		}
	}
	sort.SliceStable(snap.Tasks, func(i, j int) bool {
		return snap.Tasks[i].ID < snap.Tasks[j].ID
	})
	if snap.NextID < 1 {
		snap.NextID = nextIDFromTasks(snap.Tasks)
	}
	return snap, nil
}

// Initialize creates the board ref when it does not already exist.
func (s *GitStore) Initialize(ctx context.Context) (*Snapshot, error) {
	if snap, err := s.Load(ctx); err == nil {
		return snap, nil
	} else if !errors.Is(err, ErrRefNotFound) {
		return nil, err
	}
	snap := &Snapshot{NextID: 1}
	rev, err := s.commitSnapshot(ctx, snap, "")
	if err != nil {
		return nil, err
	}
	snap.Rev = rev
	return snap, nil
}

// Mutate applies fn to a snapshot and writes it with compare-and-swap semantics.
func (s *GitStore) Mutate(ctx context.Context, fn func(*Snapshot) error) (*Snapshot, error) {
	const attempts = 5
	for range attempts {
		snap, err := s.Load(ctx)
		if err != nil {
			return nil, err
		}
		if mutateErr := fn(snap); mutateErr != nil {
			return nil, mutateErr
		}
		newRev, err := s.commitSnapshot(ctx, snap, snap.Rev)
		if err == nil {
			snap.Rev = newRev
			return snap, nil
		}
		if !errors.Is(err, gitref.ErrUpdateRef) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("updating %s: too many concurrent modifications", s.ref)
}

func (s *GitStore) readMeta(ctx context.Context, rev string, snap *Snapshot) error {
	data, err := s.repo.ReadFile(ctx, rev, "meta.json")
	if err != nil {
		return err
	}
	var m meta
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parsing meta.json: %w", err)
	}
	if m.Version != snapshotVersion {
		return fmt.Errorf("unsupported snapshot version %d", m.Version)
	}
	snap.NextID = m.NextID
	return nil
}

func (s *GitStore) readTask(ctx context.Context, rev, path string) (*task.Task, error) {
	data, err := s.repo.ReadFile(ctx, rev, path)
	if err != nil {
		return nil, err
	}
	t, err := task.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	t.File = path
	return t, nil
}

func (s *GitStore) commitSnapshot(ctx context.Context, snap *Snapshot, expectedRev string) (string, error) {
	rootTree, err := s.writeTree(ctx, snap)
	if err != nil {
		return "", err
	}
	commit, err := s.repo.CommitTree(ctx, rootTree, expectedRev, "kanban snapshot")
	if err != nil {
		return "", err
	}
	if err := s.repo.UpdateRef(ctx, s.ref, commit, expectedRev); err != nil {
		return "", err
	}
	return commit, nil
}

func (s *GitStore) writeTree(ctx context.Context, snap *Snapshot) (string, error) {
	tasksTree, err := s.writeTasksTree(ctx, snap.Tasks)
	if err != nil {
		return "", err
	}
	metaBlob, err := s.writeMeta(ctx, snap)
	if err != nil {
		return "", err
	}
	return s.repo.MakeTree(ctx, []gitref.TreeEntry{
		{Mode: "100644", Type: "blob", OID: metaBlob, Path: "meta.json"},
		{Mode: "040000", Type: "tree", OID: tasksTree, Path: "tasks"},
	})
}

func (s *GitStore) writeTasksTree(ctx context.Context, tasks []*task.Task) (string, error) {
	entries := make([]gitref.TreeEntry, 0, len(tasks))
	seen := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		data, err := task.Marshal(t)
		if err != nil {
			return "", err
		}
		oid, err := s.repo.WriteBlob(ctx, data)
		if err != nil {
			return "", err
		}
		name := taskFilename(t, seen)
		entries = append(entries, gitref.TreeEntry{Mode: "100644", Type: "blob", OID: oid, Path: name})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return s.repo.MakeTree(ctx, entries)
}

func (s *GitStore) writeMeta(ctx context.Context, snap *Snapshot) (string, error) {
	m := meta{Version: snapshotVersion, NextID: snap.NextID}
	if m.NextID < 1 {
		m.NextID = nextIDFromTasks(snap.Tasks)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling meta.json: %w", err)
	}
	data = append(data, '\n')
	return s.repo.WriteBlob(ctx, data)
}

func taskFilename(t *task.Task, seen map[string]bool) string {
	name := ""
	if strings.HasPrefix(t.File, "tasks/") {
		name = filepath.Base(t.File)
	}
	if name == "" {
		name = task.GenerateFilename(t.ID, task.GenerateSlug(t.Title))
	}
	if !seen[name] {
		seen[name] = true
		return name
	}
	name = task.GenerateFilename(t.ID, task.GenerateSlug(t.Title))
	seen[name] = true
	return name
}

func nextIDFromTasks(tasks []*task.Task) int {
	next := 1
	for _, t := range tasks {
		if t.ID >= next {
			next = t.ID + 1
		}
	}
	return next
}
