package util

import (
	"strings"
)

func SplitManifestsKeepingEmpty(bigFile string) []string {
	const sep = "\n---"

	var result []string
	for _, d := range strings.SplitAfter(bigFile, sep) {
		d = strings.TrimSuffix(d, sep)

		hasContent := false
		for _, line := range strings.Split(d, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && trimmed != "---" && !strings.HasPrefix(trimmed, "#") {
				hasContent = true
				break
			}
		}

		if hasContent {
			result = append(result, strings.TrimSpace(d)+"\n")
		} else {
			result = append(result, "")
		}
	}

	return result
}
