package utls

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aymanbagabas/go-udiff"
	"github.com/aymanbagabas/go-udiff/myers"
	"github.com/gookit/color"
	"github.com/samber/lo"
	"github.com/wI2L/jsondiff"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
)

func ColoredUnifiedDiff(from, to string) (uDiff string, present bool) {
	edits := myers.ComputeEdits(from, to)
	if len(edits) == 0 {
		return "", false
	}

	uncoloredUDiff := lo.Must1(udiff.ToUnified("", "", from, edits, udiff.DefaultContextLines))

	var uDiffLines []string
	var firstHunkHeaderStripped bool
	lines := strings.Split(uncoloredUDiff, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") || (i == len(lines)-1 && strings.TrimSpace(line) == "") {
			continue
		}

		if strings.HasPrefix(line, "@@ ") && strings.HasSuffix(line, " @@") {
			if !firstHunkHeaderStripped {
				firstHunkHeaderStripped = true
				continue
			}
			uDiffLines = append(uDiffLines, color.Gray.Renderln("   ..."))
		} else if strings.HasPrefix(line, "+") {
			uDiffLines = append(uDiffLines, color.Green.Renderln(line[:1]+" "+line[1:]))
		} else if strings.HasPrefix(line, "-") {
			uDiffLines = append(uDiffLines, color.Red.Renderln(line[:1]+" "+line[1:]))
		} else if strings.TrimSpace(line) == "" {
			uDiffLines = append(uDiffLines, color.Gray.Renderln(line))
		} else {
			uDiffLines = append(uDiffLines, color.Gray.Renderln(" "+line))
		}
	}

	if len(uDiffLines) == 0 {
		return "", false
	}

	return strings.Trim(strings.Join(uDiffLines, "\n"), "\n"), true
}

func ResourcesReallyDiffer(first, second *unstructured.Unstructured) (differ bool, err error) {
	firstJson, err := json.Marshal(first.UnstructuredContent())
	if err != nil {
		return false, fmt.Errorf("error marshalling live object: %w", err)
	}

	secondJson, err := json.Marshal(second.UnstructuredContent())
	if err != nil {
		return false, fmt.Errorf("error marshalling desired object: %w", err)
	}

	diffOps, err := jsondiff.CompareJSON(firstJson, secondJson)
	if err != nil {
		return false, fmt.Errorf("error comparing json: %w", err)
	}

	significantDiffOps := lo.Filter(diffOps, func(op jsondiff.Operation, _ int) bool {
		return !strings.HasPrefix(op.Path, "/metadata/creationTimestamp") &&
			!strings.HasPrefix(op.Path, "/metadata/generation") &&
			!strings.HasPrefix(op.Path, "/metadata/resourceVersion") &&
			!strings.HasPrefix(op.Path, "/metadata/uid") &&
			!strings.HasPrefix(op.Path, "/status") &&
			!lo.Must(regexp.MatchString(`^/metadata/managedFields/[0-9]+/time$`, op.Path)) &&
			!lo.Must(regexp.MatchString(`^/metadata/annotations/.*werf.io.*`, op.Path)) &&
			!lo.Must(regexp.MatchString(`^/metadata/annotations/helm.sh~1hook.*`, op.Path)) &&
			!lo.Must(regexp.MatchString(`^/metadata/labels/.*werf.io.*`, op.Path))
	})

	return len(significantDiffOps) > 0, nil
}
