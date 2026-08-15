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
	pendingComments := ""

	for i := 0; i < len(source.Content); i += 2 {
		key := source.Content[i]
		oldValue := source.Content[i+1]
		if _, known := knownKeys[key.Value]; !known {
			key = withLeadingComments(key, pendingComments)
			pendingComments = ""
			merged.Content = append(merged.Content, key, oldValue)
			continue
		}

		currentValue, present := canonicalPairs[key.Value]
		if !present {
			pendingComments = joinYAMLComments(pendingComments, nodeComments(key), nodeComments(oldValue))
			continue
		}
		// The field key gives a top-level value stable identity, so its anchor
		// remains bound when the typed value changes. Sequence items have no
		// stable key and are matched by semantic value below to prevent an
		// anchor from moving to a different item after removal.
		preserveNodePresentation(currentValue, oldValue)
		key = withLeadingComments(key, pendingComments)
		pendingComments = ""
		merged.Content = append(merged.Content, key, currentValue)
		seen[key.Value] = true
	}
	merged.FootComment = joinYAMLComments(pendingComments, merged.FootComment)

	for i := 0; i < len(canonical.Content); i += 2 {
		key := canonical.Content[i]
		if seen[key.Value] {
			continue
		}
		merged.Content = append(merged.Content, key, canonical.Content[i+1])
	}

	return &merged
}

func nodeComments(node *yaml.Node) string {
	comments := joinYAMLComments(node.HeadComment, node.LineComment)
	for _, child := range node.Content {
		comments = joinYAMLComments(comments, nodeComments(child))
	}
	return joinYAMLComments(comments, node.FootComment)
}

func withLeadingComments(node *yaml.Node, comments string) *yaml.Node {
	if comments == "" {
		return node
	}
	clone := *node
	clone.HeadComment = joinYAMLComments(comments, clone.HeadComment)
	return &clone
}

func joinYAMLComments(comments ...string) string {
	nonEmpty := make([]string, 0, len(comments))
	for _, comment := range comments {
		if comment != "" {
			nonEmpty = append(nonEmpty, comment)
		}
	}
	return strings.Join(nonEmpty, "\n")
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
	case yaml.DocumentNode:
		for i := 0; i < len(current.Content) && i < len(original.Content); i++ {
			preserveNodePresentation(current.Content[i], original.Content[i])
		}
	case yaml.SequenceNode:
		preserveSequencePresentation(current, original)
	case yaml.MappingNode:
		preserveMappingPresentation(current, original)
	}
}

func preserveSequencePresentation(current, original *yaml.Node) {
	if yamlNodesSemanticallyEqual(current, original) {
		for i := range current.Content {
			preserveNodePresentation(current.Content[i], original.Content[i])
		}
		return
	}

	matchedOriginal := make([]bool, len(original.Content))
	for _, currentItem := range current.Content {
		originalIndex, unique := uniqueSequenceItemMatch(current.Content, original.Content, currentItem)
		if !unique {
			continue
		}
		preserveNodePresentation(currentItem, original.Content[originalIndex])
		matchedOriginal[originalIndex] = true
	}

	for i, originalItem := range original.Content {
		if !matchedOriginal[i] {
			current.FootComment = joinYAMLComments(current.FootComment, nodeComments(originalItem))
		}
	}
}

func uniqueSequenceItemMatch(currentItems, originalItems []*yaml.Node, target *yaml.Node) (int, bool) {
	currentMatches := 0
	for _, item := range currentItems {
		if yamlNodesSemanticallyEqual(target, item) {
			currentMatches++
		}
	}
	if currentMatches != 1 {
		return 0, false
	}

	originalIndex := -1
	for i, item := range originalItems {
		if !yamlNodesSemanticallyEqual(target, item) {
			continue
		}
		if originalIndex >= 0 {
			return 0, false
		}
		originalIndex = i
	}
	return originalIndex, originalIndex >= 0
}

func yamlNodesSemanticallyEqual(left, right *yaml.Node) bool {
	if left.Kind != right.Kind || left.Tag != right.Tag || left.Value != right.Value ||
		len(left.Content) != len(right.Content) {
		return false
	}
	for i := range left.Content {
		if !yamlNodesSemanticallyEqual(left.Content[i], right.Content[i]) {
			return false
		}
	}
	return true
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
