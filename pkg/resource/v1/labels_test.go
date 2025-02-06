package v1_test

import (
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLabelsMatch(t *testing.T) {
	// Two nil labels
	a := v1.Labels(nil)
	b := v1.Labels(nil)
	require.True(t, a.Contains(b))
	require.True(t, b.Contains(a))

	// Two empty labels
	a = map[string]string{}
	b = map[string]string{}
	require.True(t, a.Contains(b))
	require.True(t, b.Contains(a))

	// Two identical labels
	a = map[string]string{"foo": "bar"}
	b = map[string]string{"foo": "bar"}
	require.True(t, a.Contains(b))
	require.True(t, b.Contains(a))

	// Supersets against nil labels
	a = v1.Labels(nil)
	b = map[string]string{"baz": "qux", "foo": "bar"}
	require.False(t, a.Contains(b))
	require.True(t, b.Contains(a))

	// Superset against empty labels
	a = map[string]string{}
	b = map[string]string{"baz": "qux", "foo": "bar"}
	require.False(t, a.Contains(b))
	require.True(t, b.Contains(a))

	// Superset against subset labels
	a = map[string]string{"foo": "bar"}
	b = map[string]string{"baz": "qux", "foo": "bar"}
	require.False(t, a.Contains(b))
	require.True(t, b.Contains(a))
}
