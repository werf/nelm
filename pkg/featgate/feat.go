package featgate

import (
	"os"

	"github.com/chanced/caps"

	"github.com/werf/nelm/internal/common"
)

const (
	// TODO(v2): always enable
	FeatGateRemoteCharts = "remote-charts"
)

var FeatGateEnvVarsPrefix = caps.ToScreamingSnake(common.Brand) + "_FEAT_"

func FeatGateEnabled(featGate string) bool {
	return os.Getenv(FeatGateEnvVarsPrefix+caps.ToScreamingSnake(featGate)) == "true"
}
