package ondiskname_test

import (
	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOnDiskNameUUID(t *testing.T) {
	onDiskNameOriginal := ondiskname.New("test-vm", uuid.New().String())

	onDiskNameParsed, err := ondiskname.Parse(onDiskNameOriginal.String())
	require.NoError(t, err)

	require.Equal(t, onDiskNameOriginal, onDiskNameParsed)
}

func TestOnDiskNameNonUUID(t *testing.T) {
	onDiskNameOriginal := ondiskname.New("some-vm", "some-uid")

	_, err := ondiskname.Parse(onDiskNameOriginal.String())
	require.Error(t, err)
}

func TestOnDiskNameNonOrchard(t *testing.T) {
	_, err := ondiskname.Parse("ghcr.io/cirruslabs/macos-ventura-base:latest")
	require.Error(t, err)
}
