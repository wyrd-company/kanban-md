package board

import "github.com/antopolskiy/kanban-md/internal/task"

// ParentCyclePath reports whether making parentID the parent of taskID would
// close a ring in the parent tree.
//
// It returns the offending chain, starting at taskID and ending back at it, so
// the caller can name the ring in an error message; nil means the resulting
// tree stays acyclic. A parent that does not exist is not a cycle — that case
// belongs to the existence check. A ring already present in the stored data
// that taskID is not part of ends the walk instead of trapping it.
func ParentCyclePath(tasks []*task.Task, taskID, parentID int) []int {
	byID := indexByID(tasks)
	if _, exists := byID[parentID]; !exists {
		return nil
	}

	path := []int{taskID}
	visited := make(map[int]bool)
	for id := parentID; ; {
		path = append(path, id)
		if id == taskID {
			return path
		}
		if visited[id] {
			return nil
		}
		visited[id] = true

		current, ok := byID[id]
		if !ok || current.Parent == nil {
			return nil
		}
		id = *current.Parent
	}
}

// DependencyCyclePath reports whether adding depID to the dependencies of
// taskID would close a ring in the depends_on graph, in which neither task
// could ever become unblocked.
//
// Unlike the parent tree a task may depend on several others, so the walk
// explores every branch and returns the first chain that leads back to taskID.
// The result starts at taskID and ends back at it; nil means the graph stays
// acyclic.
func DependencyCyclePath(tasks []*task.Task, taskID, depID int) []int {
	byID := indexByID(tasks)
	if _, exists := byID[depID]; !exists {
		return nil
	}
	return findDependencyPath(byID, taskID, depID, []int{taskID}, make(map[int]bool))
}

// findDependencyPath walks the dependencies of current depth-first, looking for
// taskID. path holds the chain walked so far; visited stops the search from
// re-entering a task, which also contains rings already in the stored data.
func findDependencyPath(
	byID map[int]*task.Task,
	taskID, current int,
	path []int,
	visited map[int]bool,
) []int {
	path = append(path, current)
	if current == taskID {
		return path
	}
	if visited[current] {
		return nil
	}
	visited[current] = true

	t, ok := byID[current]
	if !ok {
		return nil
	}
	for _, next := range t.DependsOn {
		if found := findDependencyPath(byID, taskID, next, path, visited); found != nil {
			return found
		}
	}
	return nil
}

func indexByID(tasks []*task.Task) map[int]*task.Task {
	byID := make(map[int]*task.Task, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t
	}
	return byID
}
