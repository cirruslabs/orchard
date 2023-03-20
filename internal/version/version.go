package version

import (
	"fmt"
	goversion "github.com/hashicorp/go-version"
	"runtime/debug"
)

var (
	Version     = "unknown"
	Commit      = "unknown"
	FullVersion = ""
)

//nolint:gochecknoinits
func init() {
	if Version == "unknown" {
		info, ok := debug.ReadBuildInfo()
		if ok {
			// We parse the version here for two reasons:
			// * to weed out the "(devel)" version and fallback to "unknown" instead
			//   (see https://github.com/golang/go/issues/29228 for details on when this might happen)
			// * to remove the "v" prefix from the BuildInfo's version (e.g. "v0.7.0") and thus be consistent
			//   with the binary builds, where the version string would be "0.7.0" instead
			semver, err := goversion.NewSemver(info.Main.Version)
			if err == nil {
				Version = semver.String()
			}
		}
	}

	FullVersion = fmt.Sprintf("%s-%s", Version, Commit)
}
