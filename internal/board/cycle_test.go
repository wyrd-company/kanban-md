package board_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/antopolskiy/kanban-md/internal/board"
	"github.com/antopolskiy/kanban-md/internal/config"
	"github.com/antopolskiy/kanban-md/internal/task"
)

func cyclePtr(i int) *int { return &i }

func assertChain(t *testing.T, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("chain = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chain = %v, want %v", got, want)
		}
	}
}

func TestParentCyclePathDetectsDirectCycle(t *testing.T) {
	// #1 already has #2 as its parent; making #1 the parent of #2 closes the
	// ring. The returned chain is what the error message shows the user.
	tasks := []*task.Task{{ID: 1, Parent: cyclePtr(2)}, {ID: 2}}

	assertChain(t, board.ParentCyclePath(tasks, 2, 1), []int{2, 1, 2})
}

func TestParentCyclePathDetectsDeepCycle(t *testing.T) {
	// #3 → #2 → #1; making #3 the parent of #1 closes a three-step ring.
	tasks := []*task.Task{
		{ID: 1}, {ID: 2, Parent: cyclePtr(1)}, {ID: 3, Parent: cyclePtr(2)},
	}

	assertChain(t, board.ParentCyclePath(tasks, 1, 3), []int{1, 3, 2, 1})
}

func TestParentCyclePathAllowsValidMoves(t *testing.T) {
	// Tree: #1 → {#2, #3}, #2 → #4.
	tasks := []*task.Task{
		{ID: 1},
		{ID: 2, Parent: cyclePtr(1)},
		{ID: 3, Parent: cyclePtr(1)},
		{ID: 4, Parent: cyclePtr(2)},
	}

	cases := []struct {
		name           string
		taskID, parent int
	}{
		{"moving a leaf to a sibling", 4, 3},
		{"moving a leaf to the root", 4, 1},
		{"attaching a subtree under a leaf", 3, 4},
		{"re-attaching to the current parent", 2, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := board.ParentCyclePath(tasks, tc.taskID, tc.parent); got != nil {
				t.Errorf("ParentCyclePath(%d, %d) = %v, want no cycle", tc.taskID, tc.parent, got)
			}
		})
	}
}

func TestParentCyclePathSelfReference(t *testing.T) {
	assertChain(t, board.ParentCyclePath([]*task.Task{{ID: 1}}, 1, 1), []int{1, 1})
}

func TestParentCyclePathSurvivesPreexistingCycle(t *testing.T) {
	// The stored data already holds a ring that #3 is not part of. The walk
	// must terminate instead of spinning inside it.
	tasks := []*task.Task{
		{ID: 1, Parent: cyclePtr(2)}, {ID: 2, Parent: cyclePtr(1)}, {ID: 3},
	}

	if got := board.ParentCyclePath(tasks, 3, 1); got != nil {
		t.Errorf("ParentCyclePath(3, 1) = %v, want no cycle for a task outside the ring", got)
	}
}

func TestParentCyclePathUnknownParent(t *testing.T) {
	// A parent that does not exist is a different error, reported by the
	// existence check; the cycle check must not claim a ring.
	if got := board.ParentCyclePath([]*task.Task{{ID: 1}}, 1, 99); got != nil {
		t.Errorf("ParentCyclePath(1, 99) = %v, want no cycle", got)
	}
}

func TestDependencyCyclePathDetectsDirectCycle(t *testing.T) {
	// #1 already depends on #2; making #2 depend on #1 deadlocks both.
	tasks := []*task.Task{{ID: 1, DependsOn: []int{2}}, {ID: 2}}

	assertChain(t, board.DependencyCyclePath(tasks, 2, 1), []int{2, 1, 2})
}

func TestDependencyCyclePathFollowsBranches(t *testing.T) {
	// #1 depends on #2 and #3; only the #3 branch leads back to #4.
	tasks := []*task.Task{
		{ID: 1, DependsOn: []int{2, 3}},
		{ID: 2},
		{ID: 3, DependsOn: []int{4}},
		{ID: 4},
	}

	if got := board.DependencyCyclePath(tasks, 4, 1); len(got) == 0 {
		t.Error("DependencyCyclePath(4, 1) found no cycle, want the chain through #3")
	}
	if got := board.DependencyCyclePath(tasks, 4, 2); got != nil {
		t.Errorf("DependencyCyclePath(4, 2) = %v, want no cycle", got)
	}
}

func TestDependencyCyclePathAllowsDiamond(t *testing.T) {
	// A diamond is not a cycle: #2 and #3 may both depend on #4.
	tasks := []*task.Task{
		{ID: 1, DependsOn: []int{2, 3}},
		{ID: 2, DependsOn: []int{4}},
		{ID: 3},
		{ID: 4},
	}

	if got := board.DependencyCyclePath(tasks, 3, 4); got != nil {
		t.Errorf("DependencyCyclePath(3, 4) = %v, want no cycle for a diamond", got)
	}
}

func TestDependencyCyclePathSurvivesPreexistingCycle(t *testing.T) {
	tasks := []*task.Task{
		{ID: 1, DependsOn: []int{2}}, {ID: 2, DependsOn: []int{1}}, {ID: 3},
	}

	if got := board.DependencyCyclePath(tasks, 3, 1); got != nil {
		t.Errorf("DependencyCyclePath(3, 1) = %v, want no cycle for a task outside the ring", got)
	}
}

func TestDependencyCyclePathUnknownDependency(t *testing.T) {
	if got := board.DependencyCyclePath([]*task.Task{{ID: 1}}, 1, 99); got != nil {
		t.Errorf("DependencyCyclePath(1, 99) = %v, want no cycle", got)
	}
}

func TestDependencyCyclePathSelfReference(t *testing.T) {
	assertChain(t, board.DependencyCyclePath([]*task.Task{{ID: 1}}, 1, 1), []int{1, 1})
}

func TestDependencyCyclePathSkipsMissingLink(t *testing.T) {
	// A dependency chain that runs into a deleted task ends there instead of
	// reporting a ring.
	tasks := []*task.Task{{ID: 1, DependsOn: []int{99}}, {ID: 2}}

	if got := board.DependencyCyclePath(tasks, 2, 1); got != nil {
		t.Errorf("DependencyCyclePath(2, 1) = %v, want no cycle across a missing link", got)
	}
}

func TestEditRejectsParentCycle(t *testing.T) {
	// The same rejection through the public Edit path, so the wiring in
	// validateDeps is covered and not just the detector behind it.
	cfg, _ := setupMutateBoard(t)
	writeCycleTask(t, cfg, &task.Task{ID: 1, Title: "A", Status: "backlog"})
	writeCycleTask(t, cfg, &task.Task{ID: 2, Title: "B", Status: "backlog"})

	now := time.Now()
	setParent := func(id int) func(*task.Task) (bool, error) {
		return func(tk *task.Task) (bool, error) {
			tk.Parent = &id
			return true, nil
		}
	}

	if _, err := board.Edit(cfg, 1, "", false, setParent(2), now); err != nil {
		t.Fatalf("first parent edit: %v", err)
	}

	_, err := board.Edit(cfg, 2, "", false, setParent(1), now)
	if err == nil {
		t.Fatal("closing the ring succeeded, want a rejection")
	}
	if !containsSubstring(err.Error(), "cycle") {
		t.Errorf("error = %q, want it to mention a cycle", err)
	}
	if !containsSubstring(err.Error(), "#2 → #1 → #2") {
		t.Errorf("error = %q, want it to name the ring", err)
	}
}

func TestEditRejectsDependencyCycle(t *testing.T) {
	cfg, _ := setupMutateBoard(t)
	writeCycleTask(t, cfg, &task.Task{ID: 1, Title: "A", Status: "backlog", DependsOn: []int{2}})
	writeCycleTask(t, cfg, &task.Task{ID: 2, Title: "B", Status: "backlog"})

	_, err := board.Edit(cfg, 2, "", false, func(tk *task.Task) (bool, error) {
		tk.DependsOn = []int{1}
		return true, nil
	}, time.Now())

	if err == nil {
		t.Fatal("closing the dependency ring succeeded, want a rejection")
	}
	if !containsSubstring(err.Error(), "cycle") {
		t.Errorf("error = %q, want it to mention a cycle", err)
	}
}

func TestEditWithoutLinksSkipsTheCycleCheck(t *testing.T) {
	// A task with neither parent nor dependencies must not pay for a board read.
	cfg, _ := setupMutateBoard(t)
	writeCycleTask(t, cfg, &task.Task{ID: 1, Title: "A", Status: "backlog"})

	_, err := board.Edit(cfg, 1, "", false, func(tk *task.Task) (bool, error) {
		tk.Title = "A renamed"
		return true, nil
	}, time.Now())
	if err != nil {
		t.Fatalf("plain edit was rejected: %v", err)
	}
}

func writeCycleTask(t *testing.T, cfg *config.Config, tk *task.Task) {
	t.Helper()
	path := filepath.Join(cfg.TasksPath(), task.GenerateFilename(tk.ID, tk.Title))
	if err := task.Write(path, tk); err != nil {
		t.Fatalf("writing task #%d: %v", tk.ID, err)
	}
}
