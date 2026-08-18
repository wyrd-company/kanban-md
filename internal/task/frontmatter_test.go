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
	frontmatterTestStatus    = "todo"
)

func TestWritePreservesAdditionalSemanticValues(t *testing.T) {
	path := writeRawTask(t, `---
custom_scalar: !integration 001 # presentation is not retained
custom_binary: !!binary aGVsbG8=
custom_float: 2.0
id: 1
title: &canonical Generic sample
status: todo
priority: medium
created: 2026-08-12T10:00:00Z
updated: 2026-08-12T10:00:00Z
custom_sequence:
  - &item alpha
  - *item
  - 3
  - true
  - null
custom_mapping:
  defaults: &defaults
    enabled: true
  nested:
    <<: *defaults
    references: [one, two]
custom_canonical_copy: *canonical
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
	if got := values["custom_binary"]; got != "hello" {
		t.Errorf("custom_binary = %#v, want decoded scalar value", got)
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
	if got := values["custom_canonical_copy"]; got != "Generic sample" {
		t.Errorf("custom_canonical_copy = %#v, want original aliased value", got)
	}
	if got := values["title"]; got != "Changed sample" {
		t.Errorf("title = %#v, want changed canonical value", got)
	}

	data, err := os.ReadFile(path) //nolint:gosec // test-owned temporary path
	if err != nil {
		t.Fatal(err)
	}
	for _, presentation := range []string{
		"!integration", "!!binary", "# presentation", "&item", "*item", "&canonical", "&defaults", "*defaults",
	} {
		if strings.Contains(string(data), presentation) {
			t.Errorf("written task retained YAML presentation %q:\n%s", presentation, data)
		}
	}
}

func TestWriteMaterializesAliasWhenCanonicalAnchorIsRemoved(t *testing.T) {
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
	if got := values["custom_copy"]; got != "4h" {
		t.Errorf("custom_copy = %#v, want materialized alias value", got)
	}
}

func TestWriteMaterializesTopLevelMergeValues(t *testing.T) {
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
	if got := values["merged_reference"]; got != frontmatterTestReference {
		t.Errorf("merged_reference = %#v, want materialized merge value", got)
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
  external_reference: sample-17
"<<": *defaults
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

func TestReadBoundsAdditionalAliasExpansion(t *testing.T) {
	var content strings.Builder
	content.WriteString("---\nid: 1\ntitle: Generic sample\nstatus: todo\npriority: medium\n")
	content.WriteString("created: 2026-08-12T10:00:00Z\nupdated: 2026-08-12T10:00:00Z\ncustom:\n")
	content.WriteString("  - &a0 [x, x]\n")
	for i := 1; i <= 24; i++ {
		fmt.Fprintf(&content, "  - &a%d [*a%d, *a%d]\n", i, i-1, i-1)
	}
	content.WriteString("---\n")
	path := writeRawTask(t, content.String())

	done := make(chan error, 1)
	go func() {
		_, err := Read(path)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "excessive aliasing") {
			t.Fatalf("Read() error = %v, want bounded excessive-aliasing error", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Read() did not bound additional-property alias expansion")
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
	if got := values["custom_value"]; got != "retained" {
		t.Errorf("custom_value = %#v, want retained", got)
	}
}

func TestReadRejectsAdditionalMappingWithNonStringKey(t *testing.T) {
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

	_, err := Read(path)
	if err == nil {
		t.Fatal("Read() accepted an additional mapping with a non-string key")
	}
	if !strings.Contains(err.Error(), "custom_mapping.nested") ||
		!strings.Contains(err.Error(), "non-string mapping key at line 9") {
		t.Fatalf("Read() error = %v, want property path, location, and supported boundary", err)
	}
}

func TestReadRejectsAdditionalPropertyWithNonStringKey(t *testing.T) {
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

	_, err := Read(path)
	if err == nil {
		t.Fatal("Read() accepted an additional property with a non-string key")
	}
	if !strings.Contains(err.Error(), "key at line 7 must be a string") {
		t.Fatalf("Read() error = %v, want location and supported boundary", err)
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
