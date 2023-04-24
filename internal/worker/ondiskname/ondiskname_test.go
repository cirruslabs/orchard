package ondiskname_test

import (
	"fmt"
	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOnDiskNameFromStaticString(t *testing.T) {
	uuid := uuid.New().String()

	parsedOnDiskName, err := ondiskname.Parse(fmt.Sprintf("orchard-vm-name-%s-42", uuid))
	require.NoError(t, err)
	require.Equal(t, ondiskname.OnDiskName{"vm-name", uuid, 42}, parsedOnDiskName)
}

func TestOnDiskNameUUID(t *testing.T) {
	onDiskNameOriginal := ondiskname.New("test-vm--", uuid.New().String(), 0)

	onDiskNameParsed, err := ondiskname.Parse(onDiskNameOriginal.String())
	require.NoError(t, err)

	require.Equal(t, onDiskNameOriginal, onDiskNameParsed)
}

func TestOnDiskNameNonUUID(t *testing.T) {
	onDiskNameOriginal := ondiskname.New("some-vm", "some-uid", 0)

	_, err := ondiskname.Parse(onDiskNameOriginal.String())
	require.Error(t, err)
}

func TestOnDiskNameNonOrchard(t *testing.T) {
	_, err := ondiskname.Parse("ghcr.io/cirruslabs/macos-ventura-base:latest")
	require.Error(t, err)
}
