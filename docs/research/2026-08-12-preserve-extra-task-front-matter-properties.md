---
relationships:
  references:
    - internal/task/task.go
    - internal/task/file.go
    - internal/board/mutate.go
    - internal/task/consistency.go
    - internal/tui/board.go
---

# Preserve extra task front-matter properties

## Acceptance boundary

[Issue 16](https://github.com/antopolskiy/kanban-md/issues/16) establishes the
need: tools that share task files must be able to add properties without a
later kanban-md mutation deleting them.

The accepted boundary is semantic YAML values:

- preserve additional properties composed of scalars, lists, and recursively
  string-keyed maps;
- keep kanban-md fields typed and authoritative;
- reserialize the complete front matter as YAML after a mutation; and
- do not preserve formatting, comments, key order, anchors, aliases, or custom
  tags.

Unknown properties remain file-only. Table, compact, and JSON output continue
to expose only the typed task contract.

## Observed data-loss mechanism

`task.Read` previously decoded front matter into the closed `Task` struct. The
YAML decoder ignored unknown mapping keys, and every mutation serialized that
struct through `task.Write`. The first write therefore removed all values that
did not have a field on `Task`.

The loss is centralized in `task.Write`, but it affects every path that reads
and rewrites an existing task:

| Surface | Persistence path |
| --- | --- |
| Edit, claim, release, block, unblock | `board.Edit` → `task.WriteAndRename` → `task.Write` |
| Move | `board.Move` → `task.Write` |
| Archive and delete | `board.Archive` or `board.Delete` → `task.Write` |
| Handoff | `board.Handoff` → `task.Write` |
| Pick and claim | `board.PickAndClaim` → `task.Write` |
| Terminal User Interface mutations | Shared board mutations or `task.Write` |
| Consistency repair | `repairFilenameMismatches` → `task.Write` |

`create` has no prior front matter to preserve. Read-only commands do not
normally write, but configuration loading can trigger consistency repair.

## Storage design

`Task` retains an unexported `map[string]any` containing decoded values for
properties it does not own. Custom YAML marshal and unmarshal methods keep this
state inside `internal/task`:

1. Decode the mapping into the typed task fields.
2. Derive the canonical key set from the `Task` YAML tags.
3. Validate and decode every additional value.
4. Encode current canonical values from the typed task.
5. Encode and append the additional values.

The unexported map is absent from JSON output. A task constructed in memory has
no additional values and produces the same canonical YAML as before.

This design retains values rather than syntax nodes. YAML aliases are decoded
to their referenced values before storage. Encoding therefore cannot create an
orphan alias when a canonical field is removed. Comments, anchors, tags,
styles, and source order have no retained representation.

## Validation boundary

Additional values may contain:

- YAML scalar nodes, including null;
- sequences whose items recursively satisfy this boundary; and
- mappings whose keys are strings and whose values recursively satisfy this
  boundary.

Non-string mapping keys and recursive aliases are rejected with an error that
identifies the additional property. Duplicate YAML keys remain invalid through
the YAML decoder. Canonical keys never enter the additional-property map, so a
future typed field with the same name becomes authoritative automatically.

## Compatibility and verification

No task-format version bump or migration is required. Canonical field names and
types do not change, and old files without additional properties retain their
existing behavior.

The verification set covers:

- scalar, sequence, null, and nested string-keyed map values;
- aliases to additional and canonical values after materialization;
- normalization of comments, anchors, aliases, and custom tags;
- rejection of non-string nested map keys;
- optional canonical field removal and addition;
- absence from JSON, compact, and table output;
- edit and title rename, move, claim, handoff, archive, consistency repair, and
  Terminal User Interface mutation paths; and
- the version 1 compatibility fixture.

The repository has no `CONTRIBUTING.md` or pull request template. Maintainer
pull requests use conventional commits, user-visible summaries, explicit
validation commands, edge-case tests, README updates, and issue references.
