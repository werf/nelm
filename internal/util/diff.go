package util

import (
	"strings"

	"github.com/aymanbagabas/go-udiff"
	"github.com/aymanbagabas/go-udiff/myers"
	"github.com/gookit/color"
	"github.com/samber/lo"
)

func ColoredUnifiedDiff(from, to string, diffContextLines int) string {
	edits := myers.ComputeEdits(from, to)
	if len(edits) == 0 {
		return ""
	}

	uncoloredUDiff := lo.Must1(udiff.ToUnified("", "", from, edits, diffContextLines))

	var (
		uDiffLines              []string
		firstHunkHeaderStripped bool
	)

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
		return ""
	}

	return strings.Trim(strings.Join(uDiffLines, "\n"), "\n")
}
