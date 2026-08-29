// ---
// relationships:
//   references: 2026-08-12-preserve-extra-task-front-matter-properties
// ---

package task

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

var errTaskFrontmatterNotMapping = errors.New("task frontmatter must be a YAML mapping")

const yamlStringTag = "!!str"

var canonicalTaskYAMLKeys = makeTaskYAMLKeys()

// taskYAML avoids recursively invoking Task's YAML methods while encoding and
// decoding kanban-md-owned fields.
type taskYAML Task

// UnmarshalYAML decodes kanban-md-owned fields and retains supported properties
// kanban-md does not own.
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
	decoded := make(map[string]any)
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		keyValue, supported := supportedExtraStringKey(key)
		if !supported {
			continue
		}
		if _, known := canonicalTaskYAMLKeys[keyValue]; known {
			continue
		}
		value := mapping.Content[i+1]
		if !isSupportedExtraValue(value) {
			continue
		}
		var decodedValue any
		if err := value.Decode(&decodedValue); err != nil {
			return nil, fmt.Errorf("decoding additional frontmatter property %s: %w", keyValue, err)
		}
		decoded[keyValue] = decodedValue
	}
	return decoded, nil
}

func supportedExtraStringKey(node *yaml.Node) (string, bool) {
	if node == nil || node.Kind != yaml.ScalarNode || hasUnsupportedYAMLSyntax(node) {
		return "", false
	}
	return node.Value, node.ShortTag() == yamlStringTag
}

func isSupportedExtraValue(node *yaml.Node) bool {
	if node == nil || hasUnsupportedYAMLSyntax(node) {
		return false
	}
	switch node.Kind {
	case yaml.ScalarNode:
		return true
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if !isSupportedExtraValue(item) {
				return false
			}
		}
		return true
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			if _, supported := supportedExtraStringKey(node.Content[i]); !supported ||
				!isSupportedExtraValue(node.Content[i+1]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func hasUnsupportedYAMLSyntax(node *yaml.Node) bool {
	return node.Anchor != "" || node.Style&yaml.TaggedStyle != 0 ||
		node.Tag != "" && !strings.HasPrefix(node.Tag, "!!") &&
			!strings.HasPrefix(node.Tag, "tag:yaml.org,2002:")
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

func encodeCanonicalTask(t *Task) (*yaml.Node, error) {
	var encoded yaml.Node
	if err := encoded.Encode((*taskYAML)(t)); err != nil {
		return nil, err
	}
	return taskFrontmatterMapping(&encoded)
}

func encodeExtraProperties(extra map[string]any) (*yaml.Node, error) {
	var encoded yaml.Node
	if err := encoded.Encode(prepareExtraValue(extra)); err != nil {
		return nil, fmt.Errorf("encoding additional frontmatter properties: %w", err)
	}
	mapping, err := taskFrontmatterMapping(&encoded)
	if err != nil {
		return nil, err
	}
	quoteLiteralMergeKeys(mapping)
	return mapping, nil
}

func prepareExtraValue(value any) any {
	switch typed := value.(type) {
	case float64:
		return preservedYAMLFloat(typed)
	case []any:
		prepared := make([]any, len(typed))
		for i, item := range typed {
			prepared[i] = prepareExtraValue(item)
		}
		return prepared
	case map[string]any:
		prepared := make(map[string]any, len(typed))
		for key, item := range typed {
			prepared[key] = prepareExtraValue(item)
		}
		return prepared
	default:
		return value
	}
}

type preservedYAMLFloat float64

func (value preservedYAMLFloat) MarshalYAML() (any, error) {
	encoded := strconv.FormatFloat(float64(value), 'g', -1, 64)
	switch encoded {
	case "+Inf":
		encoded = ".inf"
	case "-Inf":
		encoded = "-.inf"
	case "NaN":
		encoded = ".nan"
	default:
		if !strings.ContainsAny(encoded, ".eE") {
			encoded += ".0"
		}
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: encoded}, nil
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
