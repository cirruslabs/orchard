package worker

import (
	"testing"

	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/samber/mo"
	"github.com/stretchr/testify/require"
)

// TestExplicitStateTransitions ensures that all state transitions
// yield a defined action (something other than ActionUndefined).
func TestExplicitStateTransitions(t *testing.T) {
	possibleStates := []mo.Option[v1.VMStatus]{
		mo.None[v1.VMStatus](),
		mo.Some(v1.VMStatusPending),
		mo.Some(v1.VMStatusRunning),
		mo.Some(v1.VMStatusFailed),
	}

	for _, remote := range possibleStates {
		for _, local := range possibleStates {
			require.Positivef(t, transitions[remote][local], "state transition %s -> %s is not defined",
				optionToString(remote), optionToString(local))
		}
	}
}
