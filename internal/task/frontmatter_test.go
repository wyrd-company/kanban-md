// ---
// relationships:
//   verifies: frontmatter
// ---

package task

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

const frontmatterTestStatus = "todo"

func TestWritePreservesUnknownFrontmatterNodesAndOrder(t *testing.T) {
	path := writeRawTask(t, `---
custom_anchor: &shared
  enabled: true
id: &canonical 1
custom_alias: *shared
title: Generic sample
# integration-owned note
custom_scalar: !!str 001 # keep this comment
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
custom_sequence:
  - alpha
  - beta
custom_canonical_alias: *canonical
---

Body
`)

	tk, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	tk.Title = "Changed sample"
	if err = Write(path, tk); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	mapping := readFrontmatterNode(t, path)
	keys := mappingKeyOrder(mapping)
	assertKeyBefore(t, keys, "custom_anchor", "custom_alias")
	assertKeyBefore(t, keys, "id", "custom_canonical_alias")

	if got := mappingValue(t, mapping, "title").Value; got != "Changed sample" {
		t.Errorf("title = %q, want %q", got, "Changed sample")
	}
	if got := mappingValue(t, mapping, "custom_scalar"); got.Tag != "!!str" || got.Value != "001" {
		t.Errorf("custom_scalar = tag %q value %q, want !!str 001", got.Tag, got.Value)
	} else if got.LineComment != "# keep this comment" {
		t.Errorf("custom_scalar line comment = %q", got.LineComment)
	}
	if got := mappingKey(t, mapping, "custom_scalar").HeadComment; got != "# integration-owned note" {
		t.Errorf("custom_scalar head comment = %q", got)
	}
	if got := mappingValue(t, mapping, "custom_anchor"); got.Anchor != "shared" {
		t.Errorf("custom_anchor anchor = %q, want shared", got.Anchor)
	}
	if got := mappingValue(t, mapping, "id"); got.Anchor != "canonical" {
		t.Errorf("id anchor = %q, want canonical", got.Anchor)
	}
	if got := mappingValue(t, mapping, "custom_sequence"); got.Kind != yaml.SequenceNode || len(got.Content) != 2 {
		t.Errorf("custom_sequence was not preserved: %#v", got)
	}
}

func TestWriteRefusesToOrphanUnknownAliasBeforeChangingFile(t *testing.T) {
	path := writeRawTask(t, `---
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
estimate: &shared 4h
custom_copy: *shared
---
`)
	before, err := os.ReadFile(path) //nolint:gosec // test-owned temporary path
	if err != nil {
		t.Fatal(err)
	}

	tk, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	tk.Estimate = ""
	err = Write(path, tk)
	if err == nil {
		t.Fatal("Write() succeeded after removing an anchor used by an unknown alias")
	}
	if !strings.Contains(err.Error(), "validating frontmatter") ||
		!strings.Contains(err.Error(), "unknown anchor 'shared'") {
		t.Fatalf("Write() error = %v", err)
	}

	after, readErr := os.ReadFile(path) //nolint:gosec // test-owned temporary path
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(after, before) {
		t.Error("Write() changed the file after frontmatter validation failed")
	}
}

func TestWritePreservesUnknownAliasToNestedCanonicalNode(t *testing.T) {
	path := writeRawTask(t, `---
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
tags:
  - &shared alpha
  - beta
custom_copy: *shared
---
`)

	tk, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	tk.Tags[1] = "gamma"
	if err = Write(path, tk); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	mapping := readFrontmatterNode(t, path)
	tags := mappingValue(t, mapping, "tags")
	if got := tags.Content[0].Anchor; got != "shared" {
		t.Errorf("first tag anchor = %q, want shared", got)
	}
	if got := tags.Content[1].Value; got != "gamma" {
		t.Errorf("second tag = %q, want gamma", got)
	}
}

func TestWriteRefusesToRetargetUnknownAliasAfterNestedCanonicalRemoval(t *testing.T) {
	path := writeRawTask(t, `---
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
tags:
  - &shared alpha
  - beta
custom_copy: *shared
---
`)
	before, err := os.ReadFile(path) //nolint:gosec // test-owned temporary path
	if err != nil {
		t.Fatal(err)
	}

	tk, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	tk.Tags = tk.Tags[1:]
	err = Write(path, tk)
	if err == nil {
		t.Fatal("Write() retargeted an unknown alias after removing its anchored item")
	}
	if !strings.Contains(err.Error(), "unknown anchor 'shared'") {
		t.Fatalf("Write() error = %v", err)
	}

	after, readErr := os.ReadFile(path) //nolint:gosec // test-owned temporary path
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(after, before) {
		t.Error("Write() changed the file after nested anchor validation failed")
	}
}

func TestWriteUpdatesOptionalCanonicalFieldsWithoutPromotingExtras(t *testing.T) {
	path := writeRawTask(t, `---
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
assignee: sample-user
custom_value: retained
---
`)

	tk, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	tk.Assignee = ""
	tk.Estimate = "2h"
	if err = Write(path, tk); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	mapping := readFrontmatterNode(t, path)
	if mappingHasKey(mapping, "assignee") {
		t.Error("cleared assignee remains in frontmatter")
	}
	if got := mappingValue(t, mapping, "estimate").Value; got != "2h" {
		t.Errorf("estimate = %q, want 2h", got)
	}
	if got := mappingValue(t, mapping, "custom_value").Value; got != "retained" {
		t.Errorf("custom_value = %q, want retained", got)
	}
}

func TestInMemoryTaskKeepsCanonicalYAMLAndJSON(t *testing.T) {
	now := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	tk := &Task{
		ID:       1,
		Title:    "Generic sample",
		Status:   frontmatterTestStatus,
		Priority: "medium",
		Created:  now,
		Updated:  now,
	}

	wantYAML, err := yaml.Marshal((*taskYAML)(tk))
	if err != nil {
		t.Fatal(err)
	}
	gotYAML, err := yaml.Marshal(tk)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotYAML, wantYAML) {
		t.Errorf("in-memory YAML changed:\ngot:\n%s\nwant:\n%s", gotYAML, wantYAML)
	}

	path := writeRawTask(t, `---
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
custom_value: retained
---
`)
	loaded, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "custom_value") || strings.Contains(string(encoded), "frontmatter") {
		t.Errorf("JSON exposed retained frontmatter: %s", encoded)
	}
}

func writeRawTask(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "001-generic-sample.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFrontmatterNode(t *testing.T, path string) *yaml.Node {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test-owned temporary path
	if err != nil {
		t.Fatal(err)
	}
	fm, _, err := splitFrontmatter(data)
	if err != nil {
		t.Fatal(err)
	}
	var document yaml.Node
	if err = yaml.Unmarshal(fm, &document); err != nil {
		t.Fatal(err)
	}
	mapping, err := taskFrontmatterMapping(&document)
	if err != nil {
		t.Fatal(err)
	}
	return mapping
}

func mappingKeyOrder(mapping *yaml.Node) []string {
	keys := make([]string, 0, len(mapping.Content)/2)
	for i := 0; i < len(mapping.Content); i += 2 {
		keys = append(keys, mapping.Content[i].Value)
	}
	return keys
}

func assertKeyBefore(t *testing.T, keys []string, first, second string) {
	t.Helper()
	firstIndex, secondIndex := -1, -1
	for i, key := range keys {
		switch key {
		case first:
			firstIndex = i
		case second:
			secondIndex = i
		}
	}
	if firstIndex < 0 || secondIndex < 0 || firstIndex >= secondIndex {
		t.Errorf("key order %v does not place %q before %q", keys, first, second)
	}
}

func mappingKey(t *testing.T, mapping *yaml.Node, name string) *yaml.Node {
	t.Helper()
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == name {
			return mapping.Content[i]
		}
	}
	t.Fatalf("mapping does not contain key %q", name)
	return nil
}

func mappingValue(t *testing.T, mapping *yaml.Node, name string) *yaml.Node {
	t.Helper()
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == name {
			return mapping.Content[i+1]
		}
	}
	t.Fatalf("mapping does not contain key %q", name)
	return nil
}

func mappingHasKey(mapping *yaml.Node, name string) bool {
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == name {
			return true
		}
	}
	return false
}
