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

func TestValidateNext(t *testing.T) {
	require.NoError(t, simplename.ValidateNext("ABCDEFGHIJKLMNOPQRSTUVWXYZ012345-01234567890"))
	require.NoError(t, simplename.ValidateNext("abcdefghijklmnopqrstuvwxyz-01234567890"))
	require.NoError(t, simplename.ValidateNext("vm-1"))
	require.NoError(t, simplename.ValidateNext("host-local"))
	require.NoError(t, simplename.ValidateNext("x"))

	require.Error(t, simplename.ValidateNext(".test"), "does not start with an alphanumeric character")
	require.Error(t, simplename.ValidateNext("test."), "does not end with an alphanumeric character")
	require.Error(t, simplename.ValidateNext("vm:1"), "special characters")
	require.Error(t, simplename.ValidateNext("vm_1"), "special characters")
	require.Error(t, simplename.ValidateNext("vm.1"), "special characters")
	require.Error(t, simplename.ValidateNext("vm%"), "special characters")
	require.Error(t, simplename.ValidateNext("😐"), "non-ASCII characters")
	require.Error(t, simplename.ValidateNext(""), "empty name")
	require.Error(t, simplename.ValidateNext("1234567890123456789012345678901234567890123456789012345678901234"),
		"too long")
}
