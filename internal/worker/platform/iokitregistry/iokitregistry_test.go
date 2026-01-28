package iokitregistry_test

import (
	"testing"

	"github.com/cirruslabs/orchard/internal/worker/platform/iokitregistry"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPlatformUUID(t *testing.T) {
	platformUUID, err := iokitregistry.PlatformUUID()
	require.NoError(t, err)

	_, err = uuid.Parse(platformUUID)
	require.NoError(t, err)
}
