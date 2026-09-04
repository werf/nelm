//go:build ai_tests

package plan

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/werf/nelm/pkg/common"
)

func TestAI_StageOperationID(t *testing.T) {
	for _, stage := range common.StagesOrdered {
		for _, suffix := range []string{common.StageStartSuffix, common.StageEndSuffix} {
			op := &Operation{
				Type:     OperationTypeNoop,
				Version:  OperationVersionNoop,
				Category: OperationCategoryMeta,
				Config: &OperationConfigNoop{
					OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, stage, suffix),
				},
			}

			assert.Equal(t, op.ID(), stageOperationID(stage, suffix))
		}
	}
}
