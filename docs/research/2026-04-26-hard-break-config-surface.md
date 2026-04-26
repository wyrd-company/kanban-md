# Hard-Break Config Surface Implementation Notes

## Question

What remaining hard-break work can be completed safely without reopening the broad task-file-to-ref migration that is already implemented across the main e2e flows?

## Findings

- The current implementation already initializes new boards with `storage.ref` and no workspace task files.
- The ref-backed store, mutation path, hook notification command, and release tap ownership are present.
- The most visible remaining old-storage artifact in the supported CLI surface was `kanban-md config`, which still displayed `tasks_dir` and `next_id` even though new ref-backed boards keep task snapshots in `refs/kanban/board` and store `next_id` in `meta.json`.
- Removing those keys from the config command is a low-risk hard-break step because both were already read-only from the command surface.

## Decision

Update the config command and README to expose ref-owned storage settings instead of legacy file-backed fields:

- remove `tasks_dir`
- remove config-owned `next_id`
- add `storage.ref`
- add `storage.notifications.mode`

This keeps the user-facing configuration model aligned with the ref-only fork while leaving deeper runtime compatibility scaffolding to be removed in smaller testable slices.

## Verification

- `go test ./cmd ./e2e`
