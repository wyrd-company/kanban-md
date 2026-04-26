# Hard-Break Ref-Only Completion Notes

## Context

The remaining hard-break plan in `docs/research/2026-04-26-git-ref-only-fork-plan.md` was reviewed against the current implementation. The codebase already had a Git-ref store, ref-backed init/create/show/list flows, most mutating commands ported to `store.Mutate`, hook installation, and README positioning for `refs/kanban/board`.

## Findings

- Runtime config loading still accepted file-backed boards when `tasks_dir` and config-owned `next_id` were present.
- Migration persisted old configs before validation, which could rewrite a legacy board even though the fork should fail clearly.
- Snapshot loading silently tolerated `meta.next_id` drift by advancing it in memory.
- Snapshot loading did not enforce `meta.json` presence, unique IDs, or path/frontmatter ID agreement.
- `board --watch` still described and used task-file watching; ref-backed boards need hook-triggered directory watching or polling.

## Implementation Notes

- Config schema version moved to 12 for the hard-break runtime surface.
- Loaded runtime configs now reject `tasks_dir`/`next_id` with a manual-import message before any migrated config is saved.
- Ref snapshot loading now treats missing metadata, unsupported metadata versions, duplicate task IDs, task path/frontmatter ID mismatches, and `meta.next_id` drift as malformed storage errors.
- `board --watch` now watches the board directory for hook notification writes in hook mode and polls the board ref in poll mode.
- E2E coverage now verifies that a legacy file-backed config is rejected by the built CLI.
- E2E consistency coverage now expects malformed ref snapshots to fail instead of self-healing `next_id` drift.

## Residual Notes

Some file-oriented helpers and unit-test fixtures remain for parsing/import-style tests and older unit coverage, but loaded production configs no longer expose a file-backed runtime path.
