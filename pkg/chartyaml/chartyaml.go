// Package chartyaml edits a Chart.yaml in place, changing only the fields it is asked to set,
// so that the comments, key order and formatting of the rest of the file survive.
package chartyaml

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/samber/lo"
)

const ImagesAnnotation = "helm.sh/images"

// Image is an entry of the helm.sh/images annotation (HIP-0015).
type Image struct {
	Image string
	Name  string
}

type EditOptions struct {
	AppVersion        string
	DeleteAnnotations []string
	Name              string
	// SetAnnotations values are written as literal blocks when they span several lines.
	SetAnnotations map[string]string
	Version        string
}

// Edit sets the non-empty top-level fields and the annotations given in opts in the Chart.yaml
// data. Data is returned as is when there is nothing to change.
func Edit(ctx context.Context, data []byte, opts EditOptions) ([]byte, error) {
	if opts.Name == "" && opts.Version == "" && opts.AppVersion == "" && len(opts.SetAnnotations) == 0 && len(opts.DeleteAnnotations) == 0 {
		return data, nil
	}

	file, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse Chart.yaml: %w", err)
	}

	if len(file.Docs) != 1 {
		return nil, fmt.Errorf("parse Chart.yaml: expected a single YAML document, got %d", len(file.Docs))
	}

	body, ok := file.Docs[0].Body.(*ast.MappingNode)
	if !ok {
		return nil, fmt.Errorf("parse Chart.yaml: expected a mapping at the top level")
	}

	// A flow mapping, like the JSON Chart.yaml of werf <= 3.12, can't be edited in place, so it
	// is converted to a block mapping with the same keys in the same order.
	if body.IsFlowStyle {
		var ordered yaml.MapSlice
		if err := yaml.UnmarshalWithOptions(data, &ordered, yaml.UseOrderedMap()); err != nil {
			return nil, fmt.Errorf("parse Chart.yaml: %w", err)
		}

		blockData, err := yaml.Marshal(ordered)
		if err != nil {
			return nil, fmt.Errorf("convert Chart.yaml to block style: %w", err)
		}

		if file, err = parser.ParseBytes(blockData, parser.ParseComments); err != nil {
			return nil, fmt.Errorf("parse converted Chart.yaml: %w", err)
		}

		if body, ok = file.Docs[0].Body.(*ast.MappingNode); !ok {
			return nil, fmt.Errorf("parse converted Chart.yaml: expected a mapping at the top level")
		}
	}

	for _, field := range []struct{ key, value string }{
		{key: "name", value: opts.Name},
		{key: "version", value: opts.Version},
		{key: "appVersion", value: opts.AppVersion},
	} {
		if field.value == "" {
			continue
		}

		if err := setScalar(body, field.key, field.value); err != nil {
			return nil, fmt.Errorf("set %q: %w", field.key, err)
		}
	}

	if len(opts.SetAnnotations) > 0 || len(opts.DeleteAnnotations) > 0 {
		if err := editAnnotations(body, opts.SetAnnotations, opts.DeleteAnnotations); err != nil {
			return nil, fmt.Errorf("edit annotations: %w", err)
		}
	}

	result := file.String()
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	return []byte(result), nil
}

// UpdateImages removes the entries named in removeNames from a helm.sh/images annotation value and
// appends images. The result is empty when no entries are left, and annotationValue as is when
// nothing changes. An image whose name matches a kept entry is an error.
func UpdateImages(ctx context.Context, annotationValue string, removeNames []string, images []Image) (string, error) {
	var entries []yaml.MapSlice
	if strings.TrimSpace(annotationValue) != "" {
		if err := yaml.Unmarshal([]byte(annotationValue), &entries); err != nil {
			return "", fmt.Errorf("parse %s annotation: %w", ImagesAnnotation, err)
		}
	}

	kept := lo.Reject(entries, func(entry yaml.MapSlice, _ int) bool {
		return lo.Contains(removeNames, imageEntryName(entry))
	})

	if len(kept) == len(entries) && len(images) == 0 {
		return annotationValue, nil
	}

	for _, image := range images {
		if lo.ContainsBy(kept, func(entry yaml.MapSlice) bool { return imageEntryName(entry) == image.Name }) {
			return "", fmt.Errorf("%s in Chart.yaml already has an entry %q; remove it from Chart.yaml or rename the werf image", ImagesAnnotation, image.Name)
		}

		kept = append(kept, yaml.MapSlice{
			{Key: "name", Value: image.Name},
			{Key: "image", Value: image.Image},
		})
	}

	if len(kept) == 0 {
		return "", nil
	}

	result, err := yaml.Marshal(kept)
	if err != nil {
		return "", fmt.Errorf("marshal %s annotation: %w", ImagesAnnotation, err)
	}

	return string(result), nil
}

func editAnnotations(body *ast.MappingNode, set map[string]string, deleteKeys []string) error {
	annotationsValue := findMappingValue(body, "annotations")
	if annotationsValue == nil && len(set) == 0 {
		return nil
	}

	if annotationsValue != nil {
		if mapping, ok := annotationsValue.Value.(*ast.MappingNode); !ok || mapping.IsFlowStyle || len(mapping.Values) == 0 {
			existing := map[string]string{}
			if err := yaml.NodeToValue(annotationsValue.Value, &existing); err != nil {
				return fmt.Errorf("parse annotations: %w", err)
			}

			if len(existing) == 0 && len(set) == 0 {
				return nil
			}

			var rebuilt *ast.MappingValueNode
			if len(existing) > 0 {
				var err error
				if rebuilt, err = blockAnnotations(existing); err != nil {
					return err
				}
			}

			body.Values = lo.FilterMap(body.Values, func(value *ast.MappingValueNode, _ int) (*ast.MappingValueNode, bool) {
				if value != annotationsValue {
					return value, true
				}

				return rebuilt, rebuilt != nil
			})
			annotationsValue = rebuilt
		}
	}

	for _, key := range sortedKeys(set) {
		if annotationsValue == nil {
			newAnnotations, err := parseNode("annotations:\n" + mappingValueText(key, set[key], 2))
			if err != nil {
				return err
			}

			annotationsValue = newAnnotations
			body.Values = append(body.Values, annotationsValue)

			continue
		}

		annotations := annotationsValue.Value.(*ast.MappingNode)
		indent := annotations.Values[0].Key.GetToken().Position.Column - 1

		newValue, err := parseMappingValue(key, set[key], indent)
		if err != nil {
			return err
		}

		if existing := findMappingValue(annotations, key); existing != nil {
			existing.Value = newValue.Value
		} else {
			annotations.Values = append(annotations.Values, newValue)
		}
	}

	if annotationsValue == nil {
		return nil
	}

	annotations := annotationsValue.Value.(*ast.MappingNode)
	annotations.Values = lo.Reject(annotations.Values, func(value *ast.MappingValueNode, _ int) bool {
		return lo.Contains(deleteKeys, mappingKey(value))
	})

	if len(annotations.Values) == 0 {
		body.Values = lo.Reject(body.Values, func(value *ast.MappingValueNode, _ int) bool {
			return value == annotationsValue
		})
	}

	return nil
}

func setScalar(mapping *ast.MappingNode, key, value string) error {
	newValue, err := parseMappingValue(key, value, 0)
	if err != nil {
		return err
	}

	mappingValue := findMappingValue(mapping, key)
	if mappingValue == nil {
		mapping.Values = append(mapping.Values, newValue)

		return nil
	}

	if _, ok := mappingValue.Value.(ast.ScalarNode); !ok {
		return fmt.Errorf("expected a scalar value")
	}

	if comment := mappingValue.Value.GetComment(); comment != nil {
		if err := newValue.Value.SetComment(comment); err != nil {
			return fmt.Errorf("keep comment: %w", err)
		}
	}

	mappingValue.Value = newValue.Value

	return nil
}

// blockAnnotations builds a block-style annotations mapping, so that entries can be added to it and
// removed from it.
func blockAnnotations(annotations map[string]string) (*ast.MappingValueNode, error) {
	var text strings.Builder
	text.WriteString("annotations:\n")

	for _, key := range sortedKeys(annotations) {
		text.WriteString(mappingValueText(key, annotations[key], 2))
	}

	return parseNode(text.String())
}

func findMappingValue(mapping *ast.MappingNode, key string) *ast.MappingValueNode {
	for _, value := range mapping.Values {
		if mappingKey(value) == key {
			return value
		}
	}

	return nil
}

func parseMappingValue(key, value string, indent int) (*ast.MappingValueNode, error) {
	return parseNode(mappingValueText(key, value, indent))
}

func imageEntryName(entry yaml.MapSlice) string {
	for _, item := range entry {
		if key, ok := item.Key.(string); ok && key == "name" {
			name, _ := item.Value.(string)

			return name
		}
	}

	return ""
}

func mappingKey(value *ast.MappingValueNode) string {
	var key string
	if err := yaml.NodeToValue(value.Key, &key); err != nil {
		return ""
	}

	return key
}

func mappingValueText(key, value string, indent int) string {
	prefix := strings.Repeat(" ", indent)
	keyText := strings.TrimSuffix(string(lo.Must(yaml.Marshal(key))), "\n")

	if !strings.Contains(strings.TrimSuffix(value, "\n"), "\n") {
		valueText := strings.TrimSuffix(string(lo.Must(yaml.Marshal(value))), "\n")

		return fmt.Sprintf("%s%s: %s\n", prefix, keyText, valueText)
	}

	var text strings.Builder
	fmt.Fprintf(&text, "%s%s: |", prefix, keyText)

	if !strings.HasSuffix(value, "\n") {
		text.WriteString("-")
	}

	text.WriteString("\n")

	for _, line := range strings.Split(strings.TrimSuffix(value, "\n"), "\n") {
		if line == "" {
			text.WriteString("\n")
			continue
		}

		fmt.Fprintf(&text, "%s  %s\n", prefix, line)
	}

	return text.String()
}

func parseNode(text string) (*ast.MappingValueNode, error) {
	file, err := parser.ParseBytes([]byte(text), parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse generated YAML %q: %w", text, err)
	}

	switch node := file.Docs[0].Body.(type) {
	case *ast.MappingValueNode:
		return node, nil
	case *ast.MappingNode:
		if len(node.Values) == 1 {
			return node.Values[0], nil
		}
	}

	panic(fmt.Sprintf("unexpected generated YAML %q", text))
}

func sortedKeys(m map[string]string) []string {
	keys := lo.Keys(m)
	sort.Strings(keys)

	return keys
}
