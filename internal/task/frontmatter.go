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
	"time"

	"go.yaml.in/yaml/v3"
)

var errTaskFrontmatterNotMapping = errors.New("task frontmatter must be a YAML mapping")

const yamlStringTag = "!!str"

var canonicalTaskYAMLKeys = makeTaskYAMLKeys()

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
	normalizeQuotedMergeKeys(mapping, make(map[*yaml.Node]bool))
	if err = mapping.Decode((*taskYAML)(t)); err != nil {
		return err
	}

	t.extraProperties, err = decodeExtraProperties(mapping)
	return err
}

// MarshalYAML encodes current kanban-md-owned values followed by the semantic
// values of properties kanban-md does not own. YAML presentation details from
// the input are intentionally not retained.
func (t Task) MarshalYAML() (any, error) {
	canonical, err := encodeCanonicalTask(&t)
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
	extra := make(map[string]any)
	hasMerge := false
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		if isYAMLMergeKey(key) {
			hasMerge = true
			continue
		}
		if key.Kind != yaml.ScalarNode || key.ShortTag() != yamlStringTag {
			return nil, fmt.Errorf("additional frontmatter key at line %d must be a string", key.Line)
		}
		if _, known := canonicalTaskYAMLKeys[key.Value]; known {
			continue
		}

		decoded, err := decodeExtraValue(mapping.Content[i+1])
		if err != nil {
			return nil, fmt.Errorf("additional frontmatter property %q: %w", key.Value, err)
		}
		extra[key.Value] = decoded
	}
	if hasMerge {
		var merged map[string]any
		if err := mapping.Decode(&merged); err != nil {
			return nil, fmt.Errorf("decoding merged task frontmatter: %w", err)
		}
		for key, value := range merged {
			if _, known := canonicalTaskYAMLKeys[key]; known {
				continue
			}
			if err := validateExtraValue(value); err != nil {
				return nil, fmt.Errorf("additional merged frontmatter property %q: %w", key, err)
			}
			extra[key] = value
		}
	}
	return extra, nil
}

func decodeExtraValue(node *yaml.Node) (any, error) {
	var decoded any
	if err := node.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decoding value: %w", err)
	}
	if err := validateExtraValue(decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func validateExtraValue(value any) error {
	switch typed := value.(type) {
	case nil, string, bool, int, int64, uint64, float64, time.Time:
		return nil
	case []any:
		for _, item := range typed {
			if err := validateExtraValue(item); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		for _, item := range typed {
			if err := validateExtraValue(item); err != nil {
				return err
			}
		}
		return nil
	case map[any]any:
		return errors.New("contains a mapping that does not have string keys")
	default:
		return fmt.Errorf("contains unsupported decoded value of type %T", value)
	}
}

func normalizeQuotedMergeKeys(node *yaml.Node, seen map[*yaml.Node]bool) {
	if node == nil || seen[node] {
		return
	}
	seen[node] = true
	if node.Kind == yaml.AliasNode {
		normalizeQuotedMergeKeys(node.Alias, seen)
		return
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Value == "<<" && key.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle) != 0 {
				key.Tag = yamlStringTag
			}
			normalizeQuotedMergeKeys(node.Content[i+1], seen)
		}
		return
	}
	for _, child := range node.Content {
		normalizeQuotedMergeKeys(child, seen)
	}
}

func quoteLiteralMergeKeys(node *yaml.Node) {
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Value == "<<" {
				key.Tag = yamlStringTag
				key.Style = yaml.DoubleQuotedStyle
			}
			quoteLiteralMergeKeys(node.Content[i+1])
		}
		return
	}
	for _, child := range node.Content {
		quoteLiteralMergeKeys(child)
	}
}

func isYAMLMergeKey(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Value == "<<" &&
		(node.Tag == "" || node.Tag == "!" || node.ShortTag() == "!!merge")
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
	mapping, err := taskFrontmatterMapping(&encoded)
	if err != nil {
		return nil, err
	}
	quoteLiteralMergeKeys(mapping)
	return mapping, nil
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

func makeTaskYAMLKeys() map[string]struct{} {
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
