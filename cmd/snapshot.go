package cmd

import (
	"context"
	"strconv"

	"github.com/antopolskiy/kanban-md/internal/clierr"
	"github.com/antopolskiy/kanban-md/internal/config"
	"github.com/antopolskiy/kanban-md/internal/store"
	"github.com/antopolskiy/kanban-md/internal/task"
)

func newStore(cfg *config.Config) (*store.GitStore, error) {
	return store.NewGitStore(context.Background(), cfg)
}

func loadSnapshot(cfg *config.Config) (*store.Snapshot, error) {
	st, err := newStore(cfg)
	if err != nil {
		return nil, err
	}
	return st.Load(context.Background())
}

func loadAllTasks(cfg *config.Config) ([]*task.Task, error) {
	if cfg.UsesRefStorage() {
		snap, err := loadSnapshot(cfg)
		if err != nil {
			return nil, err
		}
		if snap.Tasks == nil {
			return []*task.Task{}, nil
		}
		return snap.Tasks, nil
	}

	tasks, warnings, err := task.ReadAllLenient(cfg.TasksPath())
	if err != nil {
		return nil, err
	}
	printWarnings(warnings)
	if tasks == nil {
		tasks = []*task.Task{}
	}
	return tasks, nil
}

func findTaskInSnapshot(tasks []*task.Task, id int) (*task.Task, error) {
	for _, t := range tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, clierr.Newf(clierr.TaskNotFound, "task not found: #%d", id).
		WithDetails(map[string]any{"id": id})
}

func validateDepsInSnapshot(tasks []*task.Task, t *task.Task) error {
	seen := make(map[int]bool, len(tasks))
	for _, existing := range tasks {
		seen[existing.ID] = true
	}
	if t.Parent != nil {
		if *t.Parent == t.ID {
			return task.ValidateSelfReference(*t.Parent)
		}
		if !seen[*t.Parent] {
			return task.ValidateDependencyNotFound(*t.Parent)
		}
	}
	for _, id := range t.DependsOn {
		if id == t.ID {
			return task.ValidateSelfReference(id)
		}
		if !seen[id] {
			return task.ValidateDependencyNotFound(id)
		}
	}
	return nil
}

func enforceSnapshotWIPLimit(cfg *config.Config, tasks []*task.Task, t *task.Task, currentStatus, targetStatus string) error {
	classConf := cfg.ClassByName(t.Class)
	if classConf != nil && classConf.WIPLimit > 0 {
		count := countByClass(tasks, t.Class, t.ID)
		if count >= classConf.WIPLimit {
			return task.ValidateClassWIPExceeded(t.Class, classConf.WIPLimit, count)
		}
	}
	if classConf != nil && classConf.BypassColumnWIP {
		return nil
	}
	return checkWIPLimit(cfg, countByStatusExcluding(tasks, t.ID), targetStatus, currentStatus)
}

func countByStatusExcluding(tasks []*task.Task, excludeID int) map[string]int {
	counts := make(map[string]int)
	for _, t := range tasks {
		if t.ID == excludeID {
			continue
		}
		counts[t.Status]++
	}
	return counts
}

func dependentsInSnapshot(tasks []*task.Task, id int) []string {
	var msgs []string
	for _, t := range tasks {
		if t.Parent != nil && *t.Parent == id {
			msgs = append(msgs, "task #"+itoa(t.ID)+" ("+t.Title+") has this as parent")
		}
		for _, dep := range t.DependsOn {
			if dep == id {
				msgs = append(msgs, "task #"+itoa(t.ID)+" ("+t.Title+") depends on this task")
				break
			}
		}
	}
	return msgs
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
