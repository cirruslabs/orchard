package netconstants_test

import (
	"testing"

	"github.com/cirruslabs/orchard/internal/netconstants"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAddress(t *testing.T) {
	// Default port
	url, err := netconstants.NormalizeAddress("subdomain.example.com/some/prefix")
	require.NoError(t, err)
	require.Equal(t, "subdomain.example.com:6120", url.Host)
	require.Equal(t, "/some/prefix", url.Path)

	// Custom port
	url, err = netconstants.NormalizeAddress("subdomain.example.com:443/some/prefix")
	require.NoError(t, err)
	require.Equal(t, "subdomain.example.com:443", url.Host)
	require.Equal(t, "/some/prefix", url.Path)
}
