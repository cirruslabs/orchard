package opentelemetry_test

import (
	"context"
	"github.com/cirruslabs/orchard/internal/opentelemetry"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestConfigure(t *testing.T) {
	require.NoError(t, opentelemetry.Configure(context.Background()))
}
