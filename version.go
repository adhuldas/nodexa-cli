// Package nodexcli exists to carry the VERSION file into the binary: go:embed
// can only reach files at or below its own directory, and VERSION lives at the
// repository root next to scripts/release.sh, which reads the same file.
package nodexcli

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var versionFile string

// Version is the release version in the VERSION file, so a plain `go build`
// or `go install` reports it too. scripts/release.sh also sets main.Version
// with -ldflags; that value wins when present.
func Version() string {
	return strings.TrimSpace(versionFile)
}
