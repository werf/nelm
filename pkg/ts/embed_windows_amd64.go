//go:build embednode

package ts

import _ "embed"

//go:embed embed/windows/amd64/node.gz
var embeddedNode []byte

//go:embed embed/windows/amd64/npm.tar.gz
var embeddedNPM []byte
