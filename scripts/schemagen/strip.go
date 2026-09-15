package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

var (
	// nameMapKeywords are keywords whose value is keyed by *names* rather than by keywords, so their
	// keys must be left alone: "description" is a valid property name, PriorityClass has one.
	// dependentRequired and dependentSchemas are the draft 2020-12 split of "dependencies"; nothing
	// upstream uses them yet, but a property named "description" inside one would be dropped.
	nameMapKeywords = []string{
		"$defs",
		"definitions",
		"dependencies",
		"dependentRequired",
		"dependentSchemas",
		"patternProperties",
		"properties",
	}
	// opaqueKeywords hold instance data rather than a nested schema, so recursing into them could
	// corrupt a legitimate "description" value.
	opaqueKeywords = []string{
		"const",
		"default",
		"enum",
		"examples",
	}
)

func stripSchemaBytes(schemaBytes []byte) ([]byte, error) {
	var schema any

	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}

	strippedBytes, err := json.Marshal(stripSchema(schema))
	if err != nil {
		return nil, fmt.Errorf("encode stripped schema: %w", err)
	}

	return strippedBytes, nil
}

func stripNameMap(node any) any {
	names, ok := node.(map[string]any)
	if !ok {
		// "dependencies" values may also be arrays of property names.
		return stripSchema(node)
	}

	stripped := make(map[string]any, len(names))

	for name, subSchema := range names {
		stripped[name] = stripSchema(subSchema)
	}

	return stripped
}

// stripSchema removes "description" annotations, which are roughly 87% of the bytes of the Kubernetes
// schemas and carry no validation semantics. PriorityClass, which has a field of that name itself,
// shows what survives and what does not (16935 bytes upstream, 2314 in the bundle):
//
//	"description": {"description": "an arbitrary string ...", "type": ["string", "null"]}
//	"description": {"type": ["string", "null"]}
func stripSchema(node any) any {
	switch typedNode := node.(type) {
	case map[string]any:
		stripped := make(map[string]any, len(typedNode))

		for key, value := range typedNode {
			switch {
			case key == "description":
				continue
			case slices.Contains(opaqueKeywords, key) || strings.HasPrefix(key, "x-"):
				stripped[key] = value
			case slices.Contains(nameMapKeywords, key):
				stripped[key] = stripNameMap(value)
			default:
				stripped[key] = stripSchema(value)
			}
		}

		return stripped
	case []any:
		stripped := make([]any, 0, len(typedNode))

		for _, item := range typedNode {
			stripped = append(stripped, stripSchema(item))
		}

		return stripped
	default:
		return node
	}
}
