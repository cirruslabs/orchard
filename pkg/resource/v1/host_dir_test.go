package v1_test

import (
	"fmt"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHostDir(t *testing.T) {
	hostDir, err := v1.NewHostDirFromString("large-project:/Users/ci/src/www:ro")
	require.NoError(t, err)
	require.EqualValues(t, v1.HostDir{
		Name:     "large-project",
		Path:     "/Users/ci/src/www",
		ReadOnly: true,
	}, hostDir)
}

func TestHostDirString(t *testing.T) {
	require.EqualValues(t, "large-project:/Users/ci/src/www:ro", v1.HostDir{
		Name:     "large-project",
		Path:     "/Users/ci/src/www",
		ReadOnly: true,
	}.String())
}

// TestHostDirWithArchiveURL ensures that Orchard supports "http{,s}://" paths[1].
//
// [1]: https://github.com/cirruslabs/tart/pull/620
func TestHostDirWithArchiveURL(t *testing.T) {
	const hostDirRaw = "ghar:https://example.com/archive.tar.gz"

	hostDir, err := v1.NewHostDirFromString(hostDirRaw)
	require.NoError(t, err)
	require.EqualValues(t, v1.HostDir{
		Name:     "ghar",
		Path:     "https://example.com/archive.tar.gz",
		ReadOnly: false,
	}, hostDir)

	hostDirReadOnly, err := v1.NewHostDirFromString(fmt.Sprintf("%s:ro", hostDirRaw))
	require.NoError(t, err)
	require.EqualValues(t, v1.HostDir{
		Name:     "ghar",
		Path:     "https://example.com/archive.tar.gz",
		ReadOnly: true,
	}, hostDirReadOnly)
}
