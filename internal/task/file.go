package task

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

const fileMode = 0o600

// Read parses a task file and returns the Task with body populated.
func Read(path string) (*Task, error) {
	data, err := os.ReadFile(path) //nolint:gosec // task path from trusted source
	if err != nil {
		return nil, fmt.Errorf("reading task file: %w", err)
	}

	t, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	t.File = path

	return t, nil
}

// Parse parses task markdown bytes with YAML frontmatter.
func Parse(data []byte) (*Task, error) {
	fm, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}

	var t Task
	if err := yaml.Unmarshal(fm, &t); err != nil {
		return nil, fmt.Errorf("parsing frontmatter: %w", err)
	}
	if err := validateRequiredFields(&t); err != nil {
		return nil, fmt.Errorf("parsing frontmatter: %w", err)
	}

	t.Body = body

	return &t, nil
}

// Write serializes a task to a markdown file with YAML frontmatter.
func Write(path string, t *Task) error {
	data, err := Marshal(t)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, fileMode)
}

// Marshal serializes a task to markdown bytes with YAML frontmatter.
func Marshal(t *Task) ([]byte, error) {
	fm, err := yaml.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("marshaling frontmatter: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(fm)
	buf.WriteString("---\n")
	if t.Body != "" {
		buf.WriteString("\n")
		buf.WriteString(t.Body)
		if !strings.HasSuffix(t.Body, "\n") {
			buf.WriteString("\n")
		}
	}

	return buf.Bytes(), nil
}

// splitFrontmatter splits a markdown file into YAML frontmatter and body.
// The file must start with "---\n". Returns frontmatter bytes and body string.
func splitFrontmatter(data []byte) ([]byte, string, error) {
	content := string(data)

	if !strings.HasPrefix(content, "---\n") {
		return nil, "", errors.New("file does not start with YAML frontmatter (---)")
	}

	// Find the closing ---.
	rest := content[4:] // skip opening ---\n
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		// Check if file ends with \n---\n or \n--- at EOF.
		closingLen := len("---")
		if strings.HasSuffix(rest, "\n---") {
			idx = len(rest) - closingLen
		} else {
			return nil, "", errors.New("unclosed frontmatter (missing closing ---)")
		}
	}

	fm := rest[:idx]
	body := ""
	closingEnd := idx + len("\n---\n")
	if closingEnd < len(rest) {
		body = strings.TrimLeft(rest[closingEnd:], "\n")
	}

	return []byte(fm), body, nil
}

func validateRequiredFields(t *Task) error {
	if t.ID < 1 {
		return errors.New("missing required field: id")
	}
	if strings.TrimSpace(t.Title) == "" {
		return errors.New("missing required field: title")
	}
	if strings.TrimSpace(t.Status) == "" {
		return errors.New("missing required field: status")
	}
	return nil
}
