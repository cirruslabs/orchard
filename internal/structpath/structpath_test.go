package structpath_test

import (
	"github.com/cirruslabs/orchard/internal/structpath"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStructPath(t *testing.T) {
	target := struct {
		Name    string
		Aliases []string
	}{
		Name:    "Test",
		Aliases: []string{"Check-up", "Evaluation"},
	}

	t.Run("normal scenario", func(t *testing.T) {
		result, ok := structpath.Lookup(target, []string{"name"})
		require.True(t, ok)
		require.Equal(t, "Test", result)
	})

	t.Run("non-existent field", func(t *testing.T) {
		_, ok := structpath.Lookup(target, []string{"non-existent"})
		require.False(t, ok)
	})

	t.Run("non-string field", func(t *testing.T) {
		_, ok := structpath.Lookup(target, []string{"aliases"})
		require.False(t, ok)
	})
}
