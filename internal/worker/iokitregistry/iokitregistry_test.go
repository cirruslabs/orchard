package iokitregistry_test

import (
	"github.com/cirruslabs/orchard/internal/worker/iokitregistry"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPlatformUUID(t *testing.T) {
	platformUUID, err := iokitregistry.PlatformUUID()
	require.NoError(t, err)

	_, err = uuid.Parse(platformUUID)
	require.NoError(t, err)
}
