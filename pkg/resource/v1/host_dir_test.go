package v1_test

import (
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNewHostDirFromString(t *testing.T) {
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
