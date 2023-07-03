package v1_test

import (
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNewHostDirPolicyFromString(t *testing.T) {
	policy, err := v1.NewHostDirPolicyFromString("/Users/ci/src:ro")
	require.NoError(t, err)
	require.EqualValues(t, v1.HostDirPolicy{
		PathPrefix: "/Users/ci/src",
		ReadOnly:   true,
	}, policy)

	_, err = v1.NewHostDirPolicyFromString("/Users/ci/src:ro:something")
	require.Error(t, err)

	_, err = v1.NewHostDirPolicyFromString("/Users/ci/src:rw")
	require.Error(t, err)
}

func TestHostDirPolicyValidate(t *testing.T) {
	policy := &v1.HostDirPolicy{PathPrefix: "/Users/ci/src"}

	// Valid uses
	require.True(t, policy.Validate("/Users/ci/src", true))
	require.True(t, policy.Validate("/Users/ci/src/", true))
	require.True(t, policy.Validate("/Users/ci/src/website", true))

	// Invalid uses
	require.False(t, policy.Validate("/Users/ci/", true))
	require.False(t, policy.Validate("/Users", true))
	require.False(t, policy.Validate("/tmp", true))
	require.False(t, policy.Validate("/", true))

	// No path traversal, even within the path prefix
	require.False(t, policy.Validate("/Users/ci/src/website/../../../../../../etc/passwd", true))
	require.False(t, policy.Validate("/Users/ci/src/website/..", true))
	require.False(t, policy.Validate("/Users/ci/src/..", true))
	require.False(t, policy.Validate("/Users/ci/..", true))
	require.False(t, policy.Validate("/Users/..", true))
	require.False(t, policy.Validate("/..", true))
}

func TestHostDirPolicyValidateReadOnly(t *testing.T) {
	policy := &v1.HostDirPolicy{PathPrefix: "/Users/ci/src", ReadOnly: true}

	const desiredPath = "/Users/ci/src/website"

	// Only read-only is allowed
	require.True(t, policy.Validate(desiredPath, true))
	require.False(t, policy.Validate(desiredPath, false))
}

func TestHostDirPolicyString(t *testing.T) {
	policyRw := &v1.HostDirPolicy{PathPrefix: "/Users/ci/src"}
	require.EqualValues(t, "/Users/ci/src", policyRw.String())

	policyRo := &v1.HostDirPolicy{PathPrefix: "/Users/ci/src", ReadOnly: true}
	require.EqualValues(t, "/Users/ci/src:ro", policyRo.String())
}
