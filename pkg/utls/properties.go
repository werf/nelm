package utls

import (
	"fmt"
	"strings"

	"github.com/google/shlex"
)

func ParseProperties(input string) (map[string]any, error) {
	properties := make(map[string]any)

	rawProperties, err := shlex.Split(input)
	if err != nil {
		return nil, fmt.Errorf("error parsing properties: %w", err)
	}

	for _, prop := range rawProperties {
		if strings.Contains(prop, "=") {
			kv := strings.SplitN(prop, "=", 2)
			key := strings.ToLower(strings.TrimSpace(kv[0]))
			val := strings.TrimSpace(kv[1])
			properties[key] = val
		} else {
			if strings.HasPrefix(prop, "no") {
				key := strings.ToLower(prop[2:])
				properties[key] = false
			} else {
				key := strings.ToLower(prop)
				properties[key] = true
			}
		}
	}

	return properties, nil
}
