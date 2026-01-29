package v1_test

import (
	"testing"

	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/require"
)

func TestNewFilter(t *testing.T) {
	testCases := []struct {
		Name  string
		Input string
		Err   error
		Path  string
		Value string
	}{
		{
			Name:  "simple",
			Input: "a.b.c=value",
			Path:  "a.b.c",
			Value: "value",
		},
		{
			Name:  "value with equals",
			Input: "a.b.c=d=e",
			Path:  "a.b.c",
			Value: "d=e",
		},
		{
			Name:  "empty value",
			Input: "a.b.c=",
			Path:  "a.b.c",
		},
		{
			Name:  "missing value",
			Input: "abc",
			Err:   v1.ErrInvalidFilter,
		},
		{
			Name:  "missing path",
			Input: "=value",
			Err:   v1.ErrInvalidFilter,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			filter, err := v1.NewFilter(testCase.Input)
			require.ErrorIs(t, err, testCase.Err)
			require.Equal(t, testCase.Path, filter.Path)
			require.Equal(t, testCase.Value, filter.Value)
		})
	}
}
