package concurrentmap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeleteIf(t *testing.T) {
	cmap := NewConcurrentMap[int]()
	cmap.Store("a", 1)

	deleted := cmap.DeleteIf("a", func(value int) bool {
		return value == 1
	})
	require.True(t, deleted)

	_, ok := cmap.Load("a")
	require.False(t, ok)
}

func TestDeleteIfPredicateFalse(t *testing.T) {
	cmap := NewConcurrentMap[int]()
	cmap.Store("a", 1)

	deleted := cmap.DeleteIf("a", func(value int) bool {
		return value == 2
	})
	require.False(t, deleted)

	value, ok := cmap.Load("a")
	require.True(t, ok)
	require.Equal(t, 1, value)
}
