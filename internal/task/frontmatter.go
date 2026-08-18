// ---
// relationships:
//   references: 2026-08-12-preserve-extra-task-front-matter-properties
// ---

package task

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"go.yaml.in/yaml/v3"
)

var errTaskFrontmatterNotMapping = errors.New("task frontmatter must be a YAML mapping")

// taskYAML avoids recursively invoking Task's YAML methods while encoding and
// decoding kanban-md-owned fields.
type taskYAML Task

// UnmarshalYAML decodes kanban-md-owned fields and retains the semantic values
// of properties kanban-md does not own.
func (t *Task) UnmarshalYAML(value *yaml.Node) error {
	mapping, err := taskFrontmatterMapping(value)
	if err != nil {
		return err
	}
	if err = mapping.Decode((*taskYAML)(t)); err != nil {
		return err
	}

	t.extraProperties, err = decodeExtraProperties(mapping)
	return err
}

// MarshalYAML encodes current kanban-md-owned values followed by the semantic
// values of properties kanban-md does not own. YAML presentation details from
// the input are intentionally not retained.
func (t *Task) MarshalYAML() (any, error) {
	canonical, err := encodeCanonicalTask(t)
	if err != nil {
		return nil, err
	}
	if len(t.extraProperties) == 0 {
		return canonical, nil
	}

	extra, err := encodeExtraProperties(t.extraProperties)
	if err != nil {
		return nil, err
	}
	canonical.Content = append(canonical.Content, extra.Content...)
	return canonical, nil
}

func decodeExtraProperties(mapping *yaml.Node) (map[string]any, error) {
	knownKeys := taskYAMLKeys()
	extra := make(map[string]any)
	hasMerge := false
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		if key.Kind == yaml.ScalarNode && key.ShortTag() == "!!merge" {
			if err := validateExtraValue(mapping.Content[i+1], make(map[*yaml.Node]bool)); err != nil {
				return nil, fmt.Errorf("additional frontmatter merge: %w", err)
			}
			hasMerge = true
			continue
		}
		if key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" {
			return nil, errors.New("additional frontmatter properties must have string keys")
		}
		if _, known := knownKeys[key.Value]; known {
			continue
		}

		value := mapping.Content[i+1]
		if err := validateExtraValue(value, make(map[*yaml.Node]bool)); err != nil {
			return nil, fmt.Errorf("additional frontmatter property %q: %w", key.Value, err)
		}
		var decoded any
		if err := value.Decode(&decoded); err != nil {
			return nil, fmt.Errorf("decoding additional frontmatter property %q: %w", key.Value, err)
		}
		extra[key.Value] = decoded
	}
	if hasMerge {
		var merged map[string]any
		if err := mapping.Decode(&merged); err != nil {
			return nil, fmt.Errorf("decoding merged task frontmatter: %w", err)
		}
		for key, value := range merged {
			if _, known := knownKeys[key]; !known {
				extra[key] = value
			}
		}
	}
	return extra, nil
}

func validateExtraValue(node *yaml.Node, visiting map[*yaml.Node]bool) error {
	if node.Kind == yaml.AliasNode {
		if node.Alias == nil {
			return errors.New("contains an unresolved YAML alias")
		}
		node = node.Alias
	}
	if visiting[node] {
		return errors.New("contains a recursive YAML alias")
	}
	visiting[node] = true
	defer delete(visiting, node)

	switch node.Kind {
	case yaml.ScalarNode:
		return nil
	case yaml.SequenceNode:
		return validateExtraSequence(node, visiting)
	case yaml.MappingNode:
		return validateExtraMapping(node, visiting)
	default:
		return fmt.Errorf("contains unsupported YAML node kind %d", node.Kind)
	}
}

func validateExtraSequence(node *yaml.Node, visiting map[*yaml.Node]bool) error {
	for _, item := range node.Content {
		if err := validateExtraValue(item, visiting); err != nil {
			return err
		}
	}
	return nil
}

func validateExtraMapping(node *yaml.Node, visiting map[*yaml.Node]bool) error {
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind == yaml.ScalarNode && key.ShortTag() == "!!merge" {
			if err := validateExtraValue(node.Content[i+1], visiting); err != nil {
				return err
			}
			continue
		}
		if key.Kind == yaml.AliasNode {
			key = key.Alias
		}
		if key == nil || key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" {
			return errors.New("contains a mapping that does not have string keys")
		}
		if err := validateExtraValue(node.Content[i+1], visiting); err != nil {
			return err
		}
	}
	return nil
}

func encodeCanonicalTask(t *Task) (*yaml.Node, error) {
	var encoded yaml.Node
	if err := encoded.Encode((*taskYAML)(t)); err != nil {
		return nil, err
	}
	return taskFrontmatterMapping(&encoded)
}

func encodeExtraProperties(extra map[string]any) (*yaml.Node, error) {
	var encoded yaml.Node
	if err := encoded.Encode(extra); err != nil {
		return nil, fmt.Errorf("encoding additional frontmatter properties: %w", err)
	}
	return taskFrontmatterMapping(&encoded)
}

func taskFrontmatterMapping(node *yaml.Node) (*yaml.Node, error) {
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil, errTaskFrontmatterNotMapping
	}
	return node, nil
}

func taskYAMLKeys() map[string]struct{} {
	typ := reflect.TypeOf(taskYAML{})
	keys := make(map[string]struct{}, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("yaml")
		name := strings.Split(tag, ",")[0]
		if name != "" && name != "-" {
			keys[name] = struct{}{}
		}
	}
	return keys
}
