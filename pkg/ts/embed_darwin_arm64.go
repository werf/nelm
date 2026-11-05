//go:build embednode

package ts

import _ "embed"

//go:embed embed/darwin/arm64/node.gz
var embeddedNode []byte

//go:embed embed/darwin/arm64/npm.tar.gz
var embeddedNPM []byte
