// ---
// relationships:
//   references: 2026-08-12-preserve-extra-task-front-matter-properties
// ---

package task

import (
	"errors"
	"reflect"
	"strings"

	"go.yaml.in/yaml/v3"
)

var errTaskFrontmatterNotMapping = errors.New("task frontmatter must be a YAML mapping")

const yamlMappingPairWidth = 2

// taskYAML avoids recursively invoking Task's YAML methods while encoding and
// decoding kanban-md-owned fields.
type taskYAML Task

// UnmarshalYAML decodes kanban-md-owned fields and retains the source mapping
// used to preserve properties kanban-md does not own.
func (t *Task) UnmarshalYAML(value *yaml.Node) error {
	mapping, err := taskFrontmatterMapping(value)
	if err != nil {
		return err
	}
	if err = mapping.Decode((*taskYAML)(t)); err != nil {
		return err
	}
	t.frontmatter = mapping
	return nil
}

// MarshalYAML overlays current kanban-md-owned values on the source mapping.
// Unknown key/value nodes remain in their original order.
func (t *Task) MarshalYAML() (any, error) {
	canonical, err := encodeCanonicalTask(t)
	if err != nil {
		return nil, err
	}
	if t.frontmatter == nil {
		return canonical, nil
	}
	return mergeTaskFrontmatter(t.frontmatter, canonical), nil
}

func encodeCanonicalTask(t *Task) (*yaml.Node, error) {
	var encoded yaml.Node
	if err := encoded.Encode((*taskYAML)(t)); err != nil {
		return nil, err
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

func mergeTaskFrontmatter(source, canonical *yaml.Node) *yaml.Node {
	canonicalPairs := mappingPairsByKey(canonical)
	knownKeys := taskYAMLKeys()
	seen := make(map[string]bool, len(canonicalPairs))

	merged := *source
	merged.Content = make([]*yaml.Node, 0, len(source.Content)+len(canonical.Content))

	for i := 0; i < len(source.Content); i += 2 {
		key := source.Content[i]
		oldValue := source.Content[i+1]
		if _, known := knownKeys[key.Value]; !known {
			merged.Content = append(merged.Content, key, oldValue)
			continue
		}

		currentValue, present := canonicalPairs[key.Value]
		if !present {
			continue
		}
		preserveNodePresentation(currentValue, oldValue)
		merged.Content = append(merged.Content, key, currentValue)
		seen[key.Value] = true
	}

	for i := 0; i < len(canonical.Content); i += 2 {
		key := canonical.Content[i]
		if seen[key.Value] {
			continue
		}
		merged.Content = append(merged.Content, key, canonical.Content[i+1])
	}

	return &merged
}

func mappingPairsByKey(mapping *yaml.Node) map[string]*yaml.Node {
	pairs := make(map[string]*yaml.Node, len(mapping.Content)/yamlMappingPairWidth)
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		pairs[key.Value] = mapping.Content[i+1]
	}
	return pairs
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

func preserveNodePresentation(current, original *yaml.Node) {
	current.Anchor = original.Anchor
	current.HeadComment = original.HeadComment
	current.LineComment = original.LineComment
	current.FootComment = original.FootComment
	if current.Kind != original.Kind {
		return
	}
	current.Style = original.Style

	switch current.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for i := 0; i < len(current.Content) && i < len(original.Content); i++ {
			preserveNodePresentation(current.Content[i], original.Content[i])
		}
	case yaml.MappingNode:
		preserveMappingPresentation(current, original)
	}
}

func preserveMappingPresentation(current, original *yaml.Node) {
	for currentIndex := 0; currentIndex < len(current.Content); currentIndex += yamlMappingPairWidth {
		currentKey := current.Content[currentIndex]
		for originalIndex := 0; originalIndex < len(original.Content); originalIndex += yamlMappingPairWidth {
			originalKey := original.Content[originalIndex]
			if currentKey.Value != originalKey.Value {
				continue
			}
			preserveNodePresentation(currentKey, originalKey)
			preserveNodePresentation(current.Content[currentIndex+1], original.Content[originalIndex+1])
			break
		}
	}
}
