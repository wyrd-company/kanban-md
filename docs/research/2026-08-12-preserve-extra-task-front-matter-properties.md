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

## Upstream acceptance context

[Issue 16](https://github.com/antopolskiy/kanban-md/issues/16) confirms that
the maintainer accepts preservation of arbitrary task front-matter properties
in principle. The requested review emphasis is edge-case coverage for mutation
and repair mechanisms. This makes the shared `internal/task` writer and the
consistency-repair path part of the acceptance boundary, not only direct edit
commands.

The repository has no `CONTRIBUTING.md` or pull request template. The
maintainer-authored [PR 9](https://github.com/antopolskiy/kanban-md/pull/9) and
[PR 10](https://github.com/antopolskiy/kanban-md/pull/10) establish the visible
contribution pattern:

- conventional commit subjects with explanatory commit bodies;
- a pull request summary centered on user-visible behavior;
- an explicit list of validation commands;
- broad unit, snapshot, fuzz, and end-to-end edge-case coverage when the change
  affects stateful interaction;
- README changes in the same pull request when behavior is user-visible; and
- an issue-closing reference in the pull request description.

## Conclusion

kanban-md should retain the parsed YAML mapping on each task and merge typed
field changes back into that mapping when writing. This keeps kanban-md's
current typed task model while preserving properties it does not own.

Do not add an inline `map[string]any` or `map[string]yaml.Node` to `Task`.
The YAML library sorts inline map keys during encoding. An alias can then be
written before its anchor, turning valid input into invalid YAML.

No task-format version bump or migration is needed. The file format does not
gain a kanban-md field. The change only stops destructive rewrites of
user-owned fields.

## Observed behavior

A local reproduction created a task with these extra properties:

```yaml
external_id: sample-17 # integration-owned identifier
relationships:
  references:
    - generic-record
extension:
  enabled: true
```

`kanban-md show 1 --json` read the task without error. Running
`kanban-md edit 1 --priority high` removed all three properties. The body and
canonical task fields remained.

The mechanism is established:

1. `task.Read` splits the file and unmarshals front matter into `Task`.
2. `Task` contains only kanban-md's known properties.
3. The YAML decoder ignores unknown mapping keys.
4. Every persisted mutation marshals the resulting closed struct.

The first write therefore has no representation from which it could restore an
unknown property.

## Affected write paths

The loss is centralized in `task.Write`, but it is visible through every path
that reads and rewrites an existing task:

| Surface | Persistence path |
| --- | --- |
| Edit, claim, release, block, unblock | `board.Edit` → `task.WriteAndRename` → `task.Write` |
| Move | `board.Move` → `task.Write` |
| Archive and delete | `board.Archive` or `board.Delete` → `task.Write` |
| Handoff | `board.Handoff` → `task.Write` |
| Pick and claim | `board.PickAndClaim` → `task.Write` |
| Terminal user interface priority change | `Board.executePriorityChange` → `task.Write` |
| Terminal user interface edit, move, drag, and delete | Shared board mutations above |
| Consistency repair | `repairFilenameMismatches` → `task.Write` |

`create` also calls `task.Write`, but it starts a new task and has no prior
properties to preserve. Read-only commands do not normally rewrite tasks.
`loadConfig` can trigger a consistency repair, so a nominally read-only
command can rewrite a task whose ID and filename disagree.

## Options

### Inline extension map

Adding this field is the smallest change:

```go
ExtraProperties map[string]yaml.Node `yaml:",inline" json:"-"`
```

It preserves scalar types, nested values, explicit tags, and some comments. It
also keeps JSON output unchanged.

It does not preserve top-level key order or comments attached to keys. More
seriously, go.yaml v3 sorts inline map keys. This valid input:

```yaml
z_anchor: &shared
  enabled: true
a_alias: *shared
```

was encoded as `a_alias` followed by `z_anchor`. Parsing the result failed
with `unknown anchor 'shared' referenced`.

This option is not safe for arbitrary YAML properties.

### Decode and rewrite the whole task as an untyped mapping

A `map[string]any` avoids dropping keys, but moves type handling and validation
out of `Task`. It also loses YAML node details and makes every task consumer
perform conversions.

This replaces a useful typed model to solve a persistence concern. It is not
recommended.

### Preserve the YAML node and overlay canonical fields

Store the original mapping as unexported runtime state on `Task`. Continue
decoding canonical properties into typed fields. On write, encode those typed
fields to a fresh canonical mapping and merge them into the original mapping.

This preserves unknown key/value nodes and their order. It also keeps all
business logic typed. go.yaml's node model retains YAML tags, anchors, aliases,
styles, and comments, though the library does not promise byte-identical
formatting after encoding.

A throwaway prototype changed a typed title while preserving two unknown alias
relationships, including an unknown alias that targeted an anchor on the typed
`id` property. The merged output parsed successfully. The prototype was removed
after the check.

This is the recommended option.

## Recommended implementation

Keep the behavior inside `internal/task`; callers should not need to know that
extra properties exist.

1. Add an unexported `frontmatter *yaml.Node` field to `Task`, excluded from
   YAML and JSON.
2. Implement `UnmarshalYAML` for `Task`. Decode through a method-free alias
   to populate typed fields, then retain the source mapping node.
3. Implement `MarshalYAML` for `Task`. Encode the method-free alias to a
   canonical mapping, then merge it with retained source mapping.
4. Derive the canonical key set from `Task` YAML tags. Do not maintain a
   second handwritten field list.
5. For each original pair:
   - replace a canonical value with its current typed value;
   - omit a canonical property when its `omitempty` field is now empty;
   - retain an unknown key and value node unchanged.
6. Append canonical properties absent from the original mapping in struct
   order.
7. Preserve comments, style, and anchor name from an existing canonical node
   when replacing its value, so an unknown alias that refers to that anchor
   remains valid.

A task constructed in memory has no retained mapping. It should marshal exactly
as it does today. A task loaded from disk carries the mapping through board and
Terminal User Interface (TUI) mutations automatically.

Known fields remain authoritative. If a later kanban-md release adopts a name
that was previously an extra property, the typed field owns that name after the
upgrade. Duplicate YAML keys should remain invalid.

## Compatibility

- Existing task files remain readable without migration.
- Canonical front-matter names and types do not change.
- Unknown properties remain absent from JSON, compact, and table output. This
  avoids an unrelated output contract change.
- Already-stripped properties cannot be recovered.
- Formatting may be normalized by go.yaml. Semantic values, tags, comments,
  anchors, and aliases are the preservation target.
- The task compatibility fixture version remains v1. Add an extra-properties
  fixture to that version because the persisted format itself is unchanged.

## Test plan

### Serialization contract

Add a fixture containing:

- scalar, sequence, and nested mapping properties;
- explicit string and timestamp-like scalar tags;
- comments attached to extra keys and values;
- an anchor followed by an alias whose lexical order is reversed;
- an unknown alias referring to an anchor on a canonical property.

Read it, mutate canonical fields, write it, and parse it again. Assert the
unknown subtrees are semantically equal, their comments remain, and aliases
resolve. Assert a cleared optional canonical field is removed. Assert a newly
set optional field is appended once.

Add collision tests proving canonical fields do not enter the unknown set and
duplicate keys are still rejected. Marshal an in-memory task to prove existing
canonical output is unchanged. Marshal to JSON to prove retained front matter
does not leak.

### Mutation contract

Use a shared assertion helper against these paths:

- `board.Edit`, including title rename and claim release;
- `board.Move`, `board.Handoff`, `board.PickAndClaim`, and archive/delete;
- consistency repair for an ID/filename mismatch;
- direct TUI priority change.

Add end-to-end coverage for representative `edit`, `move`, `pick`, and
`archive` commands. Core serialization tests carry most edge cases; path tests
prove no mutation bypasses the preserving writer.

Run:

```text
go test -run Compat ./internal/config/ ./internal/task/
go test ./...
golangci-lint run ./...
```

For the implementation's load-bearing merge rules, mutation-test removal of
unknown-pair retention, canonical replacement, and known-field omission. Each
mutation must fail a named serialization test.

## Risks

The main risk is treating YAML as independent map entries when anchors and
aliases make entry order significant. Keeping the original node order avoids
that failure.

The remaining edge is an unknown alias that targets a canonical optional field
which a user then clears. Removing the canonical anchor would leave the alias
invalid. The writer should validate its merged node by marshaling and parsing it
before replacing the file, and return an actionable error instead of writing
corrupt front matter. Atomic file replacement would further ensure the existing
task survives any merge or validation failure.
