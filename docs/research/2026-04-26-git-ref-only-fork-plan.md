# Plan: git-ref-only fork

## Goal

Turn this fork into a Git-ref-native kanban tool that no longer stores board tasks as workspace files.

This fork will make a hard break from the original design:

- no file-backed task storage in `kanban/tasks/`
- no dual-backend support for end users
- no in-tool migration command required
- automatic hook installation when safe
- graceful fallback when hooks are already in use

The intended user is a Git repo owner who wants hidden board state managed inside Git rather than in the working tree.

## Product direction

This fork is no longer "plain files in your workspace". It becomes:

- Git-native board state
- atomic ref-based updates
- multi-worktree-visible board state
- live TUI refresh through hook-triggered notification when available

The new story should be:

- board data lives in a custom Git ref
- the TUI reflects changes across agents and worktrees
- the CLI manages the repo integration automatically when possible

## Non-goals

- Supporting the old file-backed board format as a runtime backend
- Preserving compatibility with old `kanban/tasks/*.md` boards
- Providing a first-class migration command
- Preserving "tasks are visible as normal files in the workspace"

Users can migrate manually or with an agent if they want old tasks imported.

## Distribution direction

This fork should also stop publishing like the upstream project.

Current upstream-oriented release config still targets:

- Homebrew tap owner: `antopolskiy`
- Homebrew tap repo: `homebrew-tap`

This fork should instead publish to:

- Homebrew tap owner: `wyrd-company`
- Homebrew tap repo: `homebrew-tools`

Publishing should use the already-available organization secret:

- `FORMULAE_PUBLISH_KEY`

The release plan should assume SSH-based formula publishing, not a personal access token flow.

## Breaking changes to embrace

These are intentional and should be treated as part of the fork identity.

### Board layout

Old:

```text
kanban/
  config.yml
  tasks/
    001-task.md
```

New:

```text
kanban/
  config.yml
```

Task files are no longer present in the workspace.

### Config model

The fork should stop treating `tasks_dir` as meaningful runtime state.

Recommended target config shape:

```yaml
version: 10
board:
  name: My Project
storage:
  ref: refs/kanban/board
  notifications:
    mode: auto
defaults:
  status: backlog
  priority: medium
  class: standard
statuses:
  - backlog
  - todo
  - name: in-progress
    require_claim: true
  - name: review
    require_claim: true
  - done
  - archived
priorities:
  - low
  - medium
  - high
  - critical
classes:
  - name: expedite
    wip_limit: 1
    bypass_column_wip: true
  - name: fixed-date
  - name: standard
  - name: intangible
claim_timeout: 1h
```

Recommended additions:

- `storage.ref`: exact custom ref to use
- `storage.notifications.mode`: `auto`, `hook`, or `poll`

Recommended removals:

- `tasks_dir`
- any behavior that assumes task files exist on disk

### Runtime expectations

The fork should assume:

- it is being run inside a Git repository
- the repository has a writable refs area
- the user accepts local hook installation unless a hook already exists

If any of these are false, the tool should fail clearly.

## Storage design

## Ref layout

Use one ref per board:

```text
refs/kanban/board
```

If multi-board-per-repo support matters later, this can grow into:

```text
refs/kanban/boards/<board-id>
```

For now, a single-board ref is simpler and matches the personal-use constraint.

## Snapshot layout

The ref should point to a commit whose tree contains:

```text
tasks/
  001-first-task.md
  002-second-task.md
meta.json
```

`tasks/*.md` should keep the current Markdown + YAML frontmatter format. That preserves:

- human-readable snapshot diffs
- easy import from old boards
- reuse of most task serialization logic

`meta.json` should contain ref-owned mutable state:

- `next_id`
- storage schema version
- maybe a board UUID
- maybe the notification protocol version if needed later

Example:

```json
{
  "version": 1,
  "next_id": 3
}
```

## Atomicity model

All board mutations should operate as snapshot transactions:

1. Read current ref OID
2. Load snapshot tree
3. Apply mutation in memory
4. Write new blobs/tree/commit
5. Update ref with compare-and-swap semantics
6. Retry on race

This is the core advantage of the ref model and should be leaned into.

## Architecture plan

## 1. Split serialization from persistence

Keep the current `internal/task` frontmatter parsing/writing logic, but stop making it the persistence layer.

Target split:

- `internal/task`
  - parse task bytes
  - serialize task bytes
  - validation helpers
  - timestamp logic
- `internal/store`
  - board snapshot interface
  - Git-ref implementation

Do not keep path-oriented APIs like `FindByID(tasksDir, id)` as the center of the model.

## 2. Introduce a snapshot-based store API

Suggested direction:

```go
type Snapshot struct {
    Tasks  []*task.Task
    NextID int
    Rev    string
}

type Store interface {
    Load(context.Context) (*Snapshot, error)
    Mutate(context.Context, func(*Snapshot) error) (*Snapshot, error)
    WatchToken(context.Context) (string, error)
}
```

Why this shape:

- `create` needs `next_id`
- `pick` should be truly atomic
- consistency repair can run as a snapshot rewrite
- `edit` and `move` can become simple in-memory task mutations

## 3. Add a Git helper layer

Avoid shelling out to `git` for every operation if possible.

Recommended internal package:

- `internal/gitref`

Responsibilities:

- locate Git dir and common dir
- resolve current ref OID
- read commit/tree/blob data
- write blobs/trees/commits
- CAS update the ref
- install or inspect hooks

If direct Go Git object work becomes too heavy, a limited CLI-backed implementation is acceptable for this fork, but it should be isolated behind this package.

## Hook and notification plan

## Hook choice

Use `reference-transaction`.

Reason:

- it is the relevant hook for local reference updates
- it is a much better fit than server-side push hooks
- it can react when the board ref is committed

## Hook installation policy

Policy for this fork:

1. On board init or first command that needs live notifications, detect the repo hook environment.
2. If no `reference-transaction` hook exists, install one automatically.
3. If one already exists, do not overwrite it.
4. If an existing hook blocks tool installation, switch the repo to notification polling mode automatically and warn the user once.

This matches the user requirement: automatic management unless a hook is already present.

## Hook implementation

The hook should stay tiny.

Recommended hook behavior:

- receive Git's hook arguments
- if the transaction state is `committed`, invoke:

```bash
kanban-md hook reference-transaction "$@"
```

The actual logic should live in the binary, not in shell script.

That keeps upgrades simpler and avoids shipping complicated generated hook bodies.

## Hook location

Do not assume `.githooks/` exists.

Instead:

- detect whether `core.hooksPath` is set
- if set, install into that path
- otherwise install into the repository's real hooks dir
- for worktrees, operate against the shared common Git dir rather than a per-worktree private `.git`

## IPC design

The hook should publish a lightweight invalidation signal.

Recommended first implementation:

- touch or rewrite a notification file in the board dir, for example:

```text
kanban/.notify
```

with contents like:

```json
{"ref":"refs/kanban/board","rev":"<new-oid>","at":"2026-04-26T12:34:56Z"}
```

Why start here:

- simple
- portable
- easy to debug
- still lets the TUI use fsnotify on a normal workspace file

That is technically file-based notification, but not file-based task storage. It is a practical bridge between Git internals and the existing watcher model.

Later, if desired, this can be replaced by a Unix socket, named pipe, or a lightweight local pub/sub process. For the fork's first version, a notify file is the fastest path.

## Polling fallback

If hook installation is skipped because a `reference-transaction` hook already exists, the TUI should fall back to polling.

Polling model:

- every N milliseconds, read the board ref OID
- reload when it changes

Recommended initial interval:

- 500ms when TUI is focused
- 1s for `board --watch`

This is not perfect, but it is a safe fallback and avoids trying to compose with arbitrary existing hooks.

## Command behavior plan

## `init`

New responsibilities:

- require running inside a Git repo
- create `kanban/config.yml`
- initialize the board ref if missing
- initialize `meta.json` with `next_id: 1`
- try to install notifications automatically
- record `storage.notifications.mode`

No `kanban/tasks/` directory should be created.

## `create`

Should:

- run as a store mutation
- allocate from `meta.json.next_id`
- write a new task blob under `tasks/<id>-<slug>.md`
- increment `next_id`
- update the ref atomically

## `show`, `list`, `board`, `metrics`, `context`

Should:

- load a snapshot from the ref
- operate on in-memory tasks only

These are the best commands to port first because they are read-only and exercise the new store cleanly.

## `edit`, `move`, `delete`, `handoff`, `pick`

These should become store mutations.

Important change:

`pick` becomes truly atomic in a way the current file-based version is not.

That is one of the nicest benefits of the fork and worth highlighting.

## `tui`

The TUI should:

- load tasks from the ref snapshot
- subscribe to notify-file events when hook mode is active
- otherwise poll the ref OID
- mutate through the store, not through task file paths

The TUI should not know whether persistence is "files" or "refs"; it should only know how to reload and mutate through the store.

## Consistency strategy

The current repo has a filesystem-oriented "self-healing" layer that repairs:

- duplicate IDs
- filename/frontmatter mismatches
- `next_id` drift

In the ref-only fork:

- duplicate IDs should be prevented transactionally
- `next_id` drift should not happen if all writes go through snapshot mutation
- filename/frontmatter mismatches can still happen if import code or bugs write malformed snapshot content

Recommendation:

- keep a lighter consistency pass on snapshot load
- repair in memory and write back only if needed
- treat malformed snapshots as explicit storage errors, not best-effort filesystem noise

## Tests plan

## Unit tests

Add focused tests for:

- snapshot load/save
- CAS retry behavior
- task import/export from Git tree blobs
- hook detection and installation decisions
- notify-file emission
- polling reload token logic

## Integration tests

Add end-to-end tests for:

- init in a Git repo
- create/list/show using ref storage
- pick atomicity under contention
- TUI reload after a ref mutation
- fallback behavior when a `reference-transaction` hook already exists

## Tests to delete or rewrite

Large parts of the current suite assume:

- task files exist on disk
- the tasks directory can be counted with `os.ReadDir`
- edits rename normal files

Those should be rewritten against the new storage model rather than shimmed forever.

Because this is a hard-break fork, it is okay to delete tests whose only purpose is preserving filesystem behavior.

## Recommended implementation phases

## Phase 1: foundation

Goal:

- make the codebase capable of ref-backed snapshots

Work:

- add `storage` config section
- remove `tasks_dir` from the active runtime model
- split task serialization from persistence
- add `internal/store`
- add `internal/gitref`
- implement snapshot loading from a Git ref

Deliverable:

- read-only commands can run from a ref snapshot

## Phase 1.5: release fork-over

Goal:

- make releases publish as this fork rather than the upstream project

Work:

- update `.goreleaser.yml` Homebrew repository target from `antopolskiy/homebrew-tap` to `wyrd-company/homebrew-tools`
- switch formula publishing configuration from token-based auth to SSH-based auth suitable for the deploy key stored in `FORMULAE_PUBLISH_KEY`
- update project URLs, homepage metadata, and any upstream module/repo references that appear in release artifacts
- update release workflow configuration so the deploy key is loaded for formula publishing
- verify the formula repository path and binary metadata still produce the desired formula name

Deliverable:

- tagged releases publish binaries from this fork and update the `wyrd-company/homebrew-tools` tap automatically

## Phase 2: write path

Goal:

- make CLI mutations work against the ref

Work:

- implement `Mutate(...)` with CAS updates
- port `create`
- port `edit`
- port `move`
- port `delete`
- port `handoff`
- port `pick`
- port consistency logic into snapshot form

Deliverable:

- full CLI behavior with no workspace task files

## Phase 3: notifications

Goal:

- restore live UX

Work:

- implement hook inspection
- implement automatic hook installation when safe
- add `kanban-md hook reference-transaction`
- implement notify-file writes
- teach TUI and `board --watch` to reload from notify events
- add polling fallback

Deliverable:

- live refresh across shells and worktrees

## Phase 4: TUI cleanup

Goal:

- remove path-based assumptions from the TUI

Work:

- replace all direct task file writes in TUI actions with store mutations
- remove watch-path logic that assumes `tasks/`
- update tests to assert ref-backed reload behavior

Deliverable:

- clean TUI abstraction with no lingering file-task assumptions

## Phase 5: fork cleanup

Goal:

- align the public product with the new architecture

Work:

- rewrite README positioning
- remove docs about plain workspace task files
- document hook behavior and fallback mode
- document manual migration expectations
- document release ownership and Homebrew installation from `wyrd-company/homebrew-tools`
- add a `doctor` or `status` command section that explains notification mode

Deliverable:

- product/documentation coherence

## Release publishing plan

## GoReleaser target

Update the Homebrew section in `.goreleaser.yml` to point at:

- owner: `wyrd-company`
- repo: `homebrew-tools`

The current file is still upstream-oriented, so this should be one of the first fork-identity changes even before the storage rewrite is complete.

## Authentication model

Use the organization secret `FORMULAE_PUBLISH_KEY` as the SSH deploy key for formula publishing.

Assumptions:

- the key is authorized to push to `wyrd-company/homebrew-tools`
- GitHub Actions can load the key into an SSH agent during the release workflow
- GoReleaser can publish the formula commit through SSH transport once the environment is prepared

The release workflow should not assume `HOMEBREW_TAP_TOKEN` remains the right credential for this fork.

## Release workflow expectations

The fork's release workflow should:

1. build and publish GitHub release artifacts
2. load `FORMULAE_PUBLISH_KEY`
3. configure Git/SSH for access to `git@github.com:wyrd-company/homebrew-tools.git`
4. run GoReleaser with the Homebrew tap pointed at that repository
5. fail clearly if formula publishing cannot authenticate

## Validation checklist

Before relying on the new release path, verify:

- a tag build updates the formula in `wyrd-company/homebrew-tools`
- the generated formula references this fork's release artifacts
- `brew install wyrd-company/tools/kanban-md` or the equivalent tap path works as expected
- reinstall and upgrade flows still install the `kbmd` symlink and completions

## Recommended command additions

These would make the fork easier to reason about.

### `kanban-md doctor`

Shows:

- Git repo detected or not
- board ref present or not
- hook mode: installed, skipped, polling
- current board ref OID
- Git common dir

### `kanban-md hook reference-transaction`

Internal command invoked by the hook shim.

Responsibilities:

- inspect transaction state
- emit notification for relevant ref changes

### `kanban-md storage status`

Shows:

- active board ref
- snapshot version
- notification mode

This is optional, but likely useful in a ref-native fork.

## Risks

## 1. Hook coexistence

The hardest operational edge is existing `reference-transaction` hooks.

Decision for this fork:

- do not overwrite
- do not try to auto-merge arbitrary scripts
- warn and fall back to polling

This keeps behavior predictable.

## 2. Git internals complexity

If direct object writing becomes cumbersome, the implementation could get bogged down.

Mitigation:

- isolate it early in `internal/gitref`
- allow a CLI-backed implementation temporarily if needed

## 3. TUI regression risk

The TUI currently depends on straightforward file reloads.

Mitigation:

- keep notification design simple at first
- use a normal notify file
- add polling fallback early

## 4. Hard break confusion

Old assumptions in docs and tests will become actively misleading.

Mitigation:

- rewrite docs early once Phase 1 is stable
- fail clearly on old board layouts instead of trying to be helpful in ambiguous ways

## Recommended first coding slice

If starting implementation now, the highest-leverage first slice is:

1. Add `storage.ref` config support
2. Add a minimal ref snapshot loader
3. Port `show` and `list` to the new loader
4. Add a tiny command to print the current board ref status

Why this slice:

- small enough to validate the core architecture
- exercises Git lookup and task decoding
- avoids the harder CAS and hook work at the very start

## Remaining hard-break implementation

The first coding slice is intentionally not the full clean break. If an implementation keeps old file-backed boards readable while the ref store is being introduced, that should be treated as temporary scaffolding, not product direction.

The clean-break target remains:

- no meaningful runtime `tasks_dir`
- no config-owned `next_id`
- no `kanban/tasks/*.md` task files in the workspace
- no dual backend exposed to users
- old file-backed boards fail clearly or require manual import outside the core command flow

To finish the hard break, implement the following work.

### 1. Make config ref-only

- Remove `tasks_dir` from the active runtime model.
- Remove `next_id` from config and keep it only in ref-owned `meta.json`.
- Make `storage.ref` required for all supported boards.
- Reject old file-backed configs with a clear manual-import message instead of silently preserving file storage.
- Update config fixtures and tests so the current supported shape is ref-only.

### 2. Port mutating commands to snapshot transactions

Every board mutation should use `store.Mutate(...)` so updates are atomic and compare-and-swap protected.

Commands to port:

- `edit`
- `move`
- `delete`
- `archive`
- `handoff`
- `pick`

Related behavior to port at the same time:

- dependency validation
- WIP and class-of-service enforcement
- claim checks and claim timeout handling
- lifecycle timestamps
- activity logging or a ref-native replacement for it

`pick` is especially important: it should become a single snapshot transaction that finds, claims, and optionally moves the task before updating the ref.

### 3. Port remaining read commands

Commands that currently read `kanban/tasks/` must load from the ref snapshot:

- `board`
- `metrics`
- `context`
- `log`
- any config/status output that still describes the old file layout

Where possible, keep shared filtering, sorting, and rendering logic by making it operate on already-loaded task slices.

### 4. Remove file persistence assumptions

Keep `internal/task` focused on Markdown parsing and serialization. Remove path-oriented APIs from the runtime command path, including:

- `FindByID(tasksDir, id)`
- `ReadAllLenient(tasksDir)`
- direct task file writes
- filename/frontmatter repair as normal startup behavior

Filesystem-oriented helpers can remain only as test/import utilities if they are clearly outside runtime storage.

### 5. Add ref-native consistency checks

Snapshot load should validate:

- unique task IDs
- valid task frontmatter
- `meta.json` schema version
- `meta.next_id` greater than every existing task ID
- path/frontmatter mismatches only as malformed snapshot errors unless a deliberate repair command is added

The ref model should prevent duplicate IDs and `next_id` drift during normal mutations, so consistency repair should be smaller and stricter than the old filesystem self-healing layer.

### 6. Implement notifications

- Inspect the repository hook environment.
- Install a tiny `reference-transaction` hook when no hook exists.
- Do not overwrite an existing hook.
- Add `kanban-md hook reference-transaction`.
- Emit `kanban/.notify` when `refs/kanban/board` changes.
- Fall back to polling when hook installation is unavailable.

### 7. Port the TUI

- Load board state from snapshots.
- Mutate through the store.
- Refresh from `kanban/.notify` when hook mode is active.
- Poll the ref OID when notification mode is `poll`.
- Remove watcher logic that assumes task files under `kanban/tasks/`.

### 8. Rewrite tests for the new product shape

- Update e2e setup so every initialized board lives inside a Git repository.
- Replace tests that inspect `kanban/tasks/*.md` with assertions against CLI behavior or ref snapshot contents.
- Add contention tests for atomic `pick`.
- Add tests for malformed snapshots.
- Add hook-installation and polling-fallback tests.
- Delete tests whose only purpose is preserving file-backed behavior.

### 9. Finish documentation cleanup

- Remove remaining "plain file tasks" language.
- Document `refs/kanban/board`, `meta.json`, and snapshot layout.
- Document notification modes.
- Explain that old file-backed boards are a manual migration/import concern, not a supported runtime backend.
- Document the release ownership and Homebrew tap path for `wyrd-company/homebrew-tools`.

## Bottom line

For this fork, a hard-break Git-ref-only design is reasonable and cleaner than trying to preserve dual semantics.

The key decisions that make it practical are:

- store task Markdown inside a snapshot commit tree
- move `next_id` into ref-owned metadata
- use `reference-transaction` for automatic notifications
- auto-install only when no hook already exists
- fall back to polling when an existing hook prevents installation

That gives the fork a coherent identity: hidden Git-native board state with live multi-worktree updates, without the complexity of supporting both storage models forever.
