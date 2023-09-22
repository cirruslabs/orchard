package simplename_test

import (
	"github.com/cirruslabs/orchard/internal/simplename"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestValidate(t *testing.T) {
	require.NoError(t, simplename.Validate("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz:-_."))
	require.NoError(t, simplename.Validate("vm-1"))
	require.NoError(t, simplename.Validate("vm_2"))
	require.NoError(t, simplename.Validate("host.local"))

	require.Error(t, simplename.Validate("vm%"), "special characters")
	require.Error(t, simplename.Validate("😐"), "non-ASCII characters")
}
