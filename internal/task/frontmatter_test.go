// ---
// relationships:
//   verifies: frontmatter
// ---

package task

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	frontmatterTestReference = "sample-17"
	frontmatterTestRetained  = "retained"
	frontmatterTestStatus    = "todo"
)

func TestWritePreservesAdditionalSemanticValues(t *testing.T) {
	path := writeRawTask(t, `---
custom_scalar: "001" # presentation is not retained
custom_float: 2.0
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
custom_sequence:
  - alpha
  - alpha
  - 3
  - true
  - null
custom_mapping:
  defaults:
    enabled: true
  nested:
    enabled: true
    references: [one, two]
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

	values := readFrontmatterValues(t, path)
	if got := values["custom_scalar"]; got != "001" {
		t.Errorf("custom_scalar = %#v, want %q", got, "001")
	}
	if got := values["custom_float"]; got != float64(2) {
		t.Errorf("custom_float = %#v (%T), want float64(2)", got, got)
	}
	wantSequence := []any{"alpha", "alpha", 3, true, nil}
	if got := values["custom_sequence"]; !reflect.DeepEqual(got, wantSequence) {
		t.Errorf("custom_sequence = %#v, want %#v", got, wantSequence)
	}
	wantMapping := map[string]any{
		"defaults": map[string]any{"enabled": true},
		"nested": map[string]any{
			"enabled":    true,
			"references": []any{"one", "two"},
		},
	}
	if got := values["custom_mapping"]; !reflect.DeepEqual(got, wantMapping) {
		t.Errorf("custom_mapping = %#v, want %#v", got, wantMapping)
	}
	if got := values["title"]; got != "Changed sample" {
		t.Errorf("title = %#v, want changed canonical value", got)
	}

	data, err := os.ReadFile(path) //nolint:gosec // test-owned temporary path
	if err != nil {
		t.Fatal(err)
	}
	for _, presentation := range []string{"# presentation"} {
		if strings.Contains(string(data), presentation) {
			t.Errorf("written task retained YAML presentation %q:\n%s", presentation, data)
		}
	}
}

func TestWriteDropsAdditionalPropertiesWithUnsupportedYAMLSyntax(t *testing.T) {
	path := writeRawTask(t, `---
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
estimate: &shared 4h
custom_copy: *shared
custom_anchor: &extra anchored
custom_tagged: !integration 001
custom_binary: !!binary aGVsbG8=
custom_sequence:
  - plain
  - &item anchored
  - *item
custom_mapping:
  nested: !integration value
custom_supported: retained # comments are discarded without dropping the value
---
`)

	tk, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	tk.Estimate = ""
	if err = Write(path, tk); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	values := readFrontmatterValues(t, path)
	if _, present := values["estimate"]; present {
		t.Error("cleared estimate remains in frontmatter")
	}
	for _, key := range []string{
		"custom_copy", "custom_anchor", "custom_tagged", "custom_binary", "custom_sequence", "custom_mapping",
	} {
		if _, present := values[key]; present {
			t.Errorf("unsupported additional property %q was preserved: %#v", key, values[key])
		}
	}
	if got := values["custom_supported"]; got != frontmatterTestRetained {
		t.Errorf("custom_supported = %#v, want retained", got)
	}
}

func TestWriteDropsTopLevelMergeValues(t *testing.T) {
	path := writeRawTask(t, `---
defaults: &defaults
  external_reference: merged
  merged_reference: sample-17
<<: *defaults
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
external_reference: explicit
---
`)

	tk, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	tk.Priority = "high"
	if err = Write(path, tk); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	values := readFrontmatterValues(t, path)
	if _, present := values["defaults"]; present {
		t.Errorf("anchored defaults property was preserved: %#v", values["defaults"])
	}
	if _, present := values["merged_reference"]; present {
		t.Errorf("merged_reference was materialized: %#v", values["merged_reference"])
	}
	if got := values["external_reference"]; got != "explicit" {
		t.Errorf("external_reference = %#v, want explicit value to override merged value", got)
	}
	data, err := os.ReadFile(path) //nolint:gosec // test-owned temporary path
	if err != nil {
		t.Fatal(err)
	}
	for _, presentation := range []string{"<<:", "&defaults", "*defaults"} {
		if strings.Contains(string(data), presentation) {
			t.Errorf("written task retained YAML merge presentation %q:\n%s", presentation, data)
		}
	}
}

func TestWritePreservesQuotedMergeKeyAcrossRepeatedWrites(t *testing.T) {
	path := writeRawTask(t, `---
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
defaults: &defaults
  external_reference: ignored
"<<":
  external_reference: sample-17
---
`)

	for i := 0; i < 2; i++ {
		tk, err := Read(path)
		if err != nil {
			t.Fatalf("Read() cycle %d error: %v", i+1, err)
		}
		tk.Priority = "high"
		if err = Write(path, tk); err != nil {
			t.Fatalf("Write() cycle %d error: %v", i+1, err)
		}
	}

	values := readFrontmatterValues(t, path)
	want := map[string]any{"external_reference": "sample-17"}
	if got := values["<<"]; !reflect.DeepEqual(got, want) {
		t.Errorf("quoted merge-key property = %#v, want %#v", got, want)
	}
	if _, merged := values["external_reference"]; merged {
		t.Errorf("quoted merge-key property was hoisted into the task: %#v", values)
	}
}

func TestWriteDropsPropertiesWithUnsupportedKeySyntax(t *testing.T) {
	path := writeRawTask(t, `---
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
key_source: &key custom_alias_key
*key: alias-key value
!integration custom_tagged_key: tagged-key value
&anchored_key custom_anchored_key: anchored-key value
!!str custom_explicit_key: explicit-key value
custom_mapping:
  *key: nested alias-key value
  !integration nested_tagged_key: nested tagged-key value
custom_supported: retained
---
`)

	tk, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if err = Write(path, tk); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	values := readFrontmatterValues(t, path)
	for _, key := range []string{
		"key_source", "custom_alias_key", "custom_tagged_key", "custom_anchored_key", "custom_explicit_key",
		"custom_mapping",
	} {
		if _, present := values[key]; present {
			t.Errorf("property with unsupported key syntax %q was preserved: %#v", key, values[key])
		}
	}
	if got := values["custom_supported"]; got != frontmatterTestRetained {
		t.Errorf("custom_supported = %#v, want retained", got)
	}

	data, err := os.ReadFile(path) //nolint:gosec // test-owned temporary path
	if err != nil {
		t.Fatal(err)
	}
	for _, presentation := range []string{"&key", "*key", "!integration"} {
		if strings.Contains(string(data), presentation) {
			t.Errorf("written task retained key presentation %q:\n%s", presentation, data)
		}
	}
}

func TestWriteDropsAdditionalAliasExpansionWithoutResolvingIt(t *testing.T) {
	var content strings.Builder
	content.WriteString("---\nid: 1\ntitle: Generic sample\nstatus: todo\npriority: medium\n")
	content.WriteString("created: 2026-08-12T10:00:00Z\nupdated: 2026-08-12T10:00:00Z\ncustom:\n")
	content.WriteString("  - &a0 [x, x]\n")
	for i := 1; i <= 24; i++ {
		fmt.Fprintf(&content, "  - &a%d [*a%d, *a%d]\n", i, i-1, i-1)
	}
	content.WriteString("---\n")
	path := writeRawTask(t, content.String())

	tk, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if err = Write(path, tk); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	values := readFrontmatterValues(t, path)
	if _, present := values["custom"]; present {
		t.Errorf("alias-backed property was preserved: %#v", values["custom"])
	}
}

func TestWriteKeepsCanonicalFieldsAuthoritative(t *testing.T) {
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

	values := readFrontmatterValues(t, path)
	if _, present := values["assignee"]; present {
		t.Error("cleared assignee remains in frontmatter")
	}
	if got := values["estimate"]; got != "2h" {
		t.Errorf("estimate = %#v, want 2h", got)
	}
	if got := values["custom_value"]; got != frontmatterTestRetained {
		t.Errorf("custom_value = %#v, want retained", got)
	}
}

func TestWriteDropsAdditionalMappingWithNonStringKey(t *testing.T) {
	path := writeRawTask(t, `---
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
custom_mapping:
  nested:
    1: unsupported
---
`)

	tk, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if err = Write(path, tk); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	values := readFrontmatterValues(t, path)
	if _, present := values["custom_mapping"]; present {
		t.Errorf("mapping with a non-string key was preserved: %#v", values["custom_mapping"])
	}
}

func TestWriteDropsAdditionalPropertyWithNonStringKey(t *testing.T) {
	path := writeRawTask(t, `---
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
1: unsupported
---
`)

	tk, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if err = Write(path, tk); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	values := readFrontmatterValues(t, path)
	if _, present := values["1"]; present {
		t.Errorf("property with a non-string key was preserved: %#v", values["1"])
	}
}

func TestInMemoryTaskKeepsCanonicalYAMLAndAdditionalPropertiesOutOfJSON(t *testing.T) {
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
	if strings.Contains(string(encoded), "custom_value") || strings.Contains(string(encoded), "extraProperties") {
		t.Errorf("JSON exposed additional frontmatter: %s", encoded)
	}
	valueYAML, err := yaml.Marshal(*loaded)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(valueYAML), "custom_value: retained") {
		t.Errorf("marshaling a Task value dropped additional frontmatter:\n%s", valueYAML)
	}
}

func TestReadRejectsDuplicateCanonicalKeys(t *testing.T) {
	path := writeRawTask(t, `---
id: 1
title: Generic sample
status: todo
status: done
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
---
`)

	_, err := Read(path)
	if err == nil {
		t.Fatal("Read() accepted duplicate canonical keys")
	}
	if !strings.Contains(err.Error(), "mapping key \"status\" already defined") {
		t.Fatalf("Read() error = %v", err)
	}
}

func TestWriteDropsAliasBackedDuplicateKey(t *testing.T) {
	path := writeRawTask(t, `---
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
key_source: &key custom_value
custom_value: first
*key: second
---
`)

	tk, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if err = Write(path, tk); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	values := readFrontmatterValues(t, path)
	if got := values["custom_value"]; got != "first" {
		t.Errorf("custom_value = %#v, want first", got)
	}
	if _, present := values["key_source"]; present {
		t.Errorf("anchored key source was preserved: %#v", values["key_source"])
	}
}

func TestWriteDropsMappingWithNestedAliasBackedDuplicateKey(t *testing.T) {
	path := writeRawTask(t, `---
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
key_source: &key custom_value
custom_mapping:
  custom_value: first
  *key: second
---
`)

	tk, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if err = Write(path, tk); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	values := readFrontmatterValues(t, path)
	if _, present := values["custom_mapping"]; present {
		t.Errorf("mapping with nested alias key was preserved: %#v", values["custom_mapping"])
	}
	if _, present := values["key_source"]; present {
		t.Errorf("anchored key source was preserved: %#v", values["key_source"])
	}
}

func TestReadRejectsDuplicateAdditionalKeys(t *testing.T) {
	path := writeRawTask(t, `---
id: 1
title: Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
custom_value: first
custom_value: second
---
`)

	_, err := Read(path)
	if err == nil {
		t.Fatal("Read() accepted duplicate additional keys")
	}
	if !strings.Contains(err.Error(), "mapping key \"custom_value\" already defined") {
		t.Fatalf("Read() error = %v", err)
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

func readFrontmatterValues(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test-owned temporary path
	if err != nil {
		t.Fatal(err)
	}
	fm, _, err := splitFrontmatter(data)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err = yaml.Unmarshal(fm, &values); err != nil {
		t.Fatal(err)
	}
	return values
}
