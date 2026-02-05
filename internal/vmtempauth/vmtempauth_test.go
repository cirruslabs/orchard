package vmtempauth_test

import (
	"errors"
	"testing"
	"time"

	"github.com/cirruslabs/orchard/internal/vmtempauth"
	"github.com/stretchr/testify/require"
)

func TestIssueAndVerify(t *testing.T) {
	signingKey, err := vmtempauth.NewSigningKey()
	require.NoError(t, err)

	now := time.Unix(1735779600, 0).UTC()

	issued, err := vmtempauth.Issue(signingKey, vmtempauth.IssueInput{
		Subject: "issuer",
		VMUID:   "vm-uid",
		VMName:  "vm-name",
		TTL:     10 * time.Minute,
		Now:     now,
	})
	require.NoError(t, err)

	claims, err := vmtempauth.Verify(signingKey, issued.Token, now.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, "issuer", claims.Subject)
	require.Equal(t, "vm-uid", claims.VMUID)
	require.Equal(t, "vm-name", claims.VMName)
	require.True(t, claims.HasScope(vmtempauth.ScopeVMPortForward))
	require.True(t, claims.HasScope(vmtempauth.ScopeVMIP))
	require.True(t, claims.HasScope(vmtempauth.ScopeVMSSHJumpbox))
	require.True(t, claims.CanAccessVM("vm-uid"))
}

func TestVerifyExpired(t *testing.T) {
	signingKey, err := vmtempauth.NewSigningKey()
	require.NoError(t, err)

	now := time.Unix(1735779600, 0).UTC()
	issued, err := vmtempauth.Issue(signingKey, vmtempauth.IssueInput{
		Subject: "issuer",
		VMUID:   "vm-uid",
		TTL:     time.Second,
		Now:     now,
	})
	require.NoError(t, err)

	_, err = vmtempauth.Verify(signingKey, issued.Token, now.Add(2*time.Second))
	require.Error(t, err)
	require.ErrorIs(t, err, vmtempauth.ErrTokenExpired)
}

func TestVerifyBadSignature(t *testing.T) {
	signingKey, err := vmtempauth.NewSigningKey()
	require.NoError(t, err)

	issued, err := vmtempauth.Issue(signingKey, vmtempauth.IssueInput{
		Subject: "issuer",
		VMUID:   "vm-uid",
		TTL:     time.Minute,
		Now:     time.Now().UTC(),
	})
	require.NoError(t, err)

	tampered := issued.Token[:len(issued.Token)-1] + "x"

	_, err = vmtempauth.Verify(signingKey, tampered, time.Now().UTC())
	require.Error(t, err)
	require.ErrorIs(t, err, vmtempauth.ErrSignatureMismatch)
}

func TestNormalizeTTL(t *testing.T) {
	defaultTTL, err := vmtempauth.NormalizeTTL(nil)
	require.NoError(t, err)
	require.Equal(t, vmtempauth.DefaultTTL, defaultTTL)

	zero := uint64(0)
	_, err = vmtempauth.NormalizeTTL(&zero)
	require.Error(t, err)
	require.True(t, errors.Is(err, vmtempauth.ErrInvalidTTL))

	tooLong := uint64(vmtempauth.MaxTTL/time.Second) + 1
	_, err = vmtempauth.NormalizeTTL(&tooLong)
	require.Error(t, err)
	require.True(t, errors.Is(err, vmtempauth.ErrInvalidTTL))
}
