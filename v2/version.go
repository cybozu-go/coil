package v2

import (
	"runtime/debug"
	"strings"
)

const version = "2.0.13"

// Version returns the semantic versioning string of Coil.
func Version() string {
	// Once https://github.com/golang/go/issues/37475 is resolved,
	// we can use debug.ReadBuildInfo.
	if false {
		info, ok := debug.ReadBuildInfo()
		if !ok || !strings.HasPrefix(info.Main.Version, "v") {
			return "(devel)"
		}
		return info.Main.Version[1:]
	}

	return version
}
