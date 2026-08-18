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
	if err := validateExtraPropertyNodes(mapping); err != nil {
		return nil, err
	}

	var decoded map[string]any
	if err := mapping.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decoding task frontmatter values: %w", err)
	}
	for key := range canonicalTaskYAMLKeys {
		delete(decoded, key)
	}
	return decoded, nil
}

func validateExtraPropertyNodes(mapping *yaml.Node) error {
	seen := make(map[*yaml.Node]bool)
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		if isYAMLMergeKey(key) {
			if err := validateExtraValueNode(mapping.Content[i+1], "<merge>", seen); err != nil {
				return err
			}
			continue
		}
		if key.Kind != yaml.ScalarNode || key.ShortTag() != yamlStringTag {
			return fmt.Errorf("additional frontmatter key at line %d must be a string", key.Line)
		}
		if _, known := canonicalTaskYAMLKeys[key.Value]; known {
			continue
		}
		if err := validateExtraValueNode(mapping.Content[i+1], key.Value, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateExtraValueNode(node *yaml.Node, path string, seen map[*yaml.Node]bool) error {
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if node == nil || seen[node] {
		return nil
	}
	seen[node] = true

	switch node.Kind {
	case yaml.ScalarNode:
		return nil
	case yaml.SequenceNode:
		for i, item := range node.Content {
			if err := validateExtraValueNode(item, fmt.Sprintf("%s[%d]", path, i), seen); err != nil {
				return err
			}
		}
		return nil
	case yaml.MappingNode:
		return validateExtraMappingNode(node, path, seen)
	default:
		return fmt.Errorf("additional frontmatter property %s contains unsupported YAML at line %d", path, node.Line)
	}
}

func validateExtraMappingNode(node *yaml.Node, path string, seen map[*yaml.Node]bool) error {
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if isYAMLMergeKey(key) {
			if err := validateExtraValueNode(node.Content[i+1], path+".<merge>", seen); err != nil {
				return err
			}
			continue
		}
		if key.Kind != yaml.ScalarNode || key.ShortTag() != yamlStringTag {
			return fmt.Errorf(
				"additional frontmatter property %s has a non-string mapping key at line %d",
				path,
				key.Line,
			)
		}
		if err := validateExtraValueNode(node.Content[i+1], path+"."+key.Value, seen); err != nil {
			return err
		}
	}
	return nil
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
