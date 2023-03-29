package ondiskname_test

import (
	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOnDiskNameSimple(t *testing.T) {
	onDiskNameOriginal := ondiskname.New("a", "b")

	onDiskNameParsed, err := ondiskname.Parse(onDiskNameOriginal.String())
	require.NoError(t, err)

	require.Equal(t, onDiskNameOriginal, onDiskNameParsed)
}

func TestOnDiskNameUUID(t *testing.T) {
	onDiskNameOriginal := ondiskname.New("test VM", uuid.New().String())

	onDiskNameParsed, err := ondiskname.Parse(onDiskNameOriginal.String())
	require.NoError(t, err)

	require.Equal(t, onDiskNameOriginal, onDiskNameParsed)
}
