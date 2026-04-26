# Research: storing tasks in a custom Git ref

## Summary

Replacing workspace task files with a custom Git ref is feasible, but it is not a small storage swap. In this fork, task persistence is a foundational filesystem assumption that reaches into the CLI, TUI, watcher, consistency repair, dependency validation, and a large portion of the test suite.

The safest direction is not "replace `tasks/` with Git plumbing in place". The safer direction is:

1. Introduce a storage abstraction while keeping the current filesystem backend unchanged.
2. Add an optional `git-ref` backend behind config.
3. Represent the ref as a snapshot commit whose tree still contains per-task Markdown files, plus board metadata needed for atomic mutation.

I would not recommend making the Git-ref backend the default until it has proven TUI behavior, migration tooling, and a clear story for cloning/fetching/pushing custom refs.

## Current project shape

This repo is still strongly optimized around plain files:

- `README.md` explicitly markets the tool as a "file-based Kanban", says "Every task is a Markdown file", and says the TUI auto-refreshes when task files change on disk.
- `internal/config/config.go` treats `tasks_dir` as a first-class board setting and resolves `TasksPath()` from the board directory.
- `internal/task/file.go` reads and writes task Markdown directly via `os.ReadFile` and `os.WriteFile`.
- `internal/task/find.go` discovers tasks by scanning the tasks directory with `os.ReadDir`, matching filename prefixes, and then falling back to frontmatter IDs.
- `internal/task/consistency.go` repairs duplicate IDs, filename mismatches, and `next_id` drift by rewriting task files in place.
- `cmd/create.go`, `cmd/edit.go`, `cmd/move.go`, `cmd/delete.go`, `cmd/handoff.go`, `cmd/pick.go`, and `cmd/show.go` all operate on filesystem paths returned by `cfg.TasksPath()` or `task.FindByID(...)`.
- `internal/tui/board.go` creates, edits, archives, reloads, and watches tasks through the same file APIs.
- `internal/watcher/watcher.go` is a thin fsnotify wrapper and expects filesystem events from real directories.
- `internal/board/log.go` writes `activity.jsonl` into the board directory, separate from task files.

The coupling is broad, not incidental. A quick grep found 300+ direct references across `cmd/`, `internal/`, and `e2e/` to `TasksPath()`, `task.FindByID(...)`, `task.ReadAllLenient(...)`, or `task.Write(...)`.

## Why a Git-ref backend is attractive

There are real upsides:

- No visible `kanban/tasks/*.md` churn in the workspace.
- Board state can live in the repository's Git database instead of the working tree.
- A ref update can be made compare-and-swap style with `update-ref old new`, which is a stronger atomic primitive than the current "read files, rewrite files" flow.
- In a repo with multiple worktrees, a custom ref naturally lives in the shared Git common dir, so all worktrees can see the same board state.

That last point is especially relevant here because the project's own workflow encourages Git worktrees.

## Major costs and constraints

### 1. This changes the product's core contract

Today the tool's value proposition is "plain files". A Git-ref backend weakens:

- direct editor access
- `grep`/shell visibility
- easy manual repair
- the current README promise

If this lands, it should be positioned as an optional backend, not a silent replacement.

### 2. Custom refs are repo-local unless users deliberately propagate them

A working tree file rides along with normal filesystem tools and normal commits if the user chooses to commit it.

A custom ref is different:

- normal clone/fetch behavior does not automatically make arbitrary custom refs part of the everyday workflow
- remotes and hosting layers may require explicit refspecs or special handling
- users will not naturally see board state in GitHub PRs, diffs, or file browsers

If the goal is "hide local board state from the workspace", this may be acceptable. If the goal is "shared invisible board state that just works everywhere", it is not.

### 3. The current watcher model does not map cleanly

The TUI and `board --watch` are built around fsnotify on:

- the tasks directory
- the board directory

With a ref backend, task mutations no longer emit task-file events. There are a few options:

- Poll the current ref OID on a timer.
- Watch Git internals in the common Git dir.
- Treat `activity.jsonl` as an invalidation signal.

Polling the ref hash is the most robust. Watching `.git/refs/...` is brittle because refs can move across different Git internals, and worktrees introduce a `git-common-dir` vs per-worktree `.git` split.

### 4. `next_id` and atomic mutation need redesign

Currently:

- task state lives in files
- `next_id` lives in `kanban/config.yml`
- create uses a `.lock` file to serialize access

If task storage moves into a ref but `next_id` stays in config, create becomes a two-store mutation:

- update the ref
- update `config.yml`

That loses the nice atomicity story. The clean solution is to move ref-managed metadata, at minimum `next_id`, into the ref snapshot too.

### 5. The current code is not structured for a storage swap

The task package currently combines two concerns:

- task serialization/parsing
- task discovery and filesystem persistence

To support a Git-ref backend cleanly, those need to be split. Otherwise the code will become a pile of special cases around path-like data that is no longer path-like.

### 6. Test impact will be large

The tests are heavily file-oriented:

- many unit tests create task files directly
- TUI tests inspect directory contents
- e2e tests assume a real `kanban/tasks` directory
- compatibility fixtures are file-based Markdown tasks

The Markdown task format can stay the same, which helps, but the test harnesses will still need a backend-aware layer.

## Recommended storage model

If we pursue this, the best fit is:

- Keep the task wire format exactly the same: one Markdown file per task with YAML frontmatter.
- Store those task files inside a Git tree, not as loose workspace files.
- Point a custom ref at a commit that represents the current board snapshot.

Example shape:

```text
refs/kanban/boards/<board-id>
  -> commit
     tree:
       tasks/
         001-first-task.md
         002-second-task.md
       meta.json
```

Where `meta.json` contains ref-owned state such as:

- `next_id`
- storage schema version
- maybe a stable board UUID if needed later

This approach preserves:

- the existing task Markdown schema
- human-readable diffs through Git plumbing
- the idea of one file per task

It also gives us a real atomic boundary:

- read current ref OID
- materialize/update snapshot
- write new tree + commit
- `update-ref <ref> <new-oid> <old-oid>`

If another process won the race, retry from the new ref tip.

## Implementation approach

### 1. Add a backend abstraction first

Do this before any Git-ref code.

Suggested direction:

- Keep `internal/task` focused on parsing/serializing a single task from/to bytes.
- Add a new package, for example `internal/store`, with an interface for board task persistence.

The interface should support snapshot-oriented mutation, not just ad hoc reads and writes. Something like:

```go
type Snapshot struct {
    Tasks  []*task.Task
    NextID int
    Rev    string
}

type Store interface {
    Load() (*Snapshot, error)
    Mutate(func(*Snapshot) error) (*Snapshot, error)
    WatchToken() (string, error)
}
```

Why snapshot mutation matters:

- create needs `next_id`
- pick should become truly atomic
- consistency repair touches multiple tasks plus metadata
- rename/delete/move operations are not single-record updates in the current model

### 2. Keep the filesystem backend as the reference implementation

The first backend should just wrap the current behavior:

- read from `cfg.TasksPath()`
- preserve `.lock`
- preserve `activity.jsonl`
- preserve existing task filenames

This lets the refactor land with minimal behavior change and keeps the current tests meaningful.

### 3. Add `git-ref` as an opt-in config backend

Possible config shape:

```yaml
version: 10
board:
  name: My Project
storage:
  backend: filesystem # or git-ref
  git_ref: refs/kanban/boards/1234abcd
tasks_dir: tasks
```

Notes:

- `tasks_dir` should remain for backward compatibility and the filesystem backend.
- the Git-ref name should be explicit, not derived from the current path
- this matters because one repo may hold multiple boards

### 4. Make watch/reload backend-aware

For the ref backend:

- stop depending on fsnotify for task changes
- poll the ref tip hash on an interval
- reload when the hash changes

This is simpler and more portable than trying to watch Git's internal ref files.

### 5. Convert high-level commands to store mutations

These commands should stop talking directly to paths:

- `create`
- `edit`
- `move`
- `delete`
- `handoff`
- `pick`
- archive flows
- TUI create/edit/delete/move actions
- consistency repair

`show`, `list`, `board`, `metrics`, and `context` can move after the read path exists.

## Migration and compatibility

### Config compatibility

Per this repo's own rules, config changes should:

1. bump `CurrentVersion`
2. add a migration in `internal/config/migrate.go`
3. add a new compat fixture directory
4. add/update compat tests

Backward-compatible migration path:

- old configs default to `storage.backend: filesystem`
- existing `tasks_dir` keeps working unchanged
- no task file frontmatter changes are required

### Task compatibility

This proposal can preserve the current task schema exactly. That is a big win.

The compatibility fixtures in `internal/task/testdata/compat/` can remain valid if the task bytes stay unchanged.

### User migration

This should be an explicit action, not an automatic silent conversion.

Recommended flow:

- add `kanban-md storage migrate --to git-ref`
- copy existing task files into the ref snapshot
- move `next_id` into `meta.json`
- leave workspace files intact until the user confirms cleanup

Rollback should be symmetrical:

- `kanban-md storage migrate --to filesystem`

## Things that get better with a ref backend

This idea is not only cost.

It could improve a few long-standing behaviors:

- `pick` can become truly atomic with ref compare-and-swap.
- Multi-step mutations can become transactional at the snapshot level.
- Worktrees can share the same hidden board state through the common Git dir.
- Workspace cleanliness improves for users who dislike generated task files.

## Things I would not do

- I would not replace the filesystem code path in place with "call Git from wherever `os.ReadFile` used to be".
- I would not make Git CLI invocation a required runtime dependency if it can be avoided.
- I would not make the ref backend the default before proving out watch behavior and migration.
- I would not derive the ref name from the current directory path alone.

## Recommended next step

If the goal is to explore this seriously, the best spike is:

1. Introduce `internal/store` with a filesystem backend only.
2. Port `show`, `list`, and `board` read paths to the abstraction.
3. Add a read-only Git-ref backend that can render tasks from a ref snapshot.
4. Only then add mutating commands, starting with `create` and `pick`.

That sequence answers the important questions in order:

- Can the codebase tolerate a storage abstraction?
- Is the TUI/watch story acceptable?
- Is Git-ref I/O pleasant enough without wrecking the single-binary experience?

## Bottom line

This is viable as an optional backend and could be a good fit for this fork if the priority is hiding board state from the workspace while keeping it attached to the repository.

It is not a cheap internal refactor. It is a product-level architectural change that touches the CLI contract, TUI refresh behavior, concurrency model, config schema, and a large test surface.

If we pursue it, we should do it as a staged backend abstraction project, not as a direct replacement of `kanban/tasks/*.md`.
