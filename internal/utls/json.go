package utls

import (
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/tidwall/sjson"
	"github.com/wI2L/jsondiff"
)

func MergeJson(mergeA, toB []byte) (result []byte, changed bool, err error) {
	ops, err := jsondiff.CompareJSON(toB, mergeA)
	if err != nil {
		return nil, false, fmt.Errorf("error comparing json: %w", err)
	}

	var addOps []jsondiff.Operation
	for _, op := range ops {
		switch t := op.Type; t {
		case jsondiff.OperationAdd:
			addOps = append(addOps, op)
		case jsondiff.OperationRemove, jsondiff.OperationReplace:
			continue
		default:
			panic(fmt.Sprintf("unexpected operation type: %s", t))
		}
	}

	if len(addOps) == 0 {
		return toB, false, nil
	}

	var opStrings []string
	for _, op := range addOps {
		opStrings = append(opStrings, op.String())
	}

	patchString := "[" + strings.Join(opStrings, ",") + "]"

	jpatch, err := jsonpatch.DecodePatch([]byte(patchString))
	if err != nil {
		return nil, false, fmt.Errorf("error decoding patch: %w", err)
	}

	result, err = jpatch.Apply(toB)
	if err != nil {
		return nil, false, fmt.Errorf("error applying patch: %w", err)
	}

	return result, true, nil
}

func SubtractJson(fromA, subtractB []byte) (result []byte, changed bool, err error) {
	ops, err := jsondiff.CompareJSON(subtractB, fromA)
	if err != nil {
		return nil, false, fmt.Errorf("error comparing json: %w", err)
	}

	var addOps []jsondiff.Operation
	for _, op := range ops {
		switch t := op.Type; t {
		case jsondiff.OperationAdd, jsondiff.OperationReplace:
			addOps = append(addOps, op)
		case jsondiff.OperationRemove:
			continue
		default:
			panic(fmt.Sprintf("unexpected operation type: %s", t))
		}
	}

	res := "{}"
	for _, op := range addOps {
		jsonPath := JsonPatchPathToJsonPath(op.Path)
		var err error
		res, err = sjson.Set(res, jsonPath, op.Value)
		if err != nil {
			return nil, false, fmt.Errorf("error setting value by jsonpath: %w", err)
		}
	}

	return []byte(res), string(fromA) != res, nil
}

func JsonPatchPathToJsonPath(path string) string {
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	path = strings.ReplaceAll(path, ".", `\.`)
	path = strings.ReplaceAll(path, ":", `\:`)
	return strings.ReplaceAll(path, "/", ".")
}
