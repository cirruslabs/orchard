package v1_test

import (
	"testing"

	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/require"
)

func TestConditionsSet(t *testing.T) {
	var conditions []v1.Condition

	// Ensure that a new condition is added
	v1.ConditionsSet(&conditions, v1.Condition{
		Type:  v1.ConditionTypeScheduled,
		State: v1.ConditionStateFalse,
	})
	require.Equal(t, []v1.Condition{
		{
			Type:  v1.ConditionTypeScheduled,
			State: v1.ConditionStateFalse,
		},
	}, conditions)

	// Ensure that an existing condition is updated
	v1.ConditionsSet(&conditions, v1.Condition{
		Type:  v1.ConditionTypeScheduled,
		State: v1.ConditionStateTrue,
	})
	require.Equal(t, []v1.Condition{
		{
			Type:  v1.ConditionTypeScheduled,
			State: v1.ConditionStateTrue,
		},
	}, conditions)

	// Ensure that other conditions can be added
	v1.ConditionsSet(&conditions, v1.Condition{
		Type:  v1.ConditionTypeRunning,
		State: v1.ConditionStateFalse,
	})
	require.Equal(t, []v1.Condition{
		{
			Type:  v1.ConditionTypeScheduled,
			State: v1.ConditionStateTrue,
		},
		{
			Type:  v1.ConditionTypeRunning,
			State: v1.ConditionStateFalse,
		},
	}, conditions)

	// Ensure that other conditions can be updated
	v1.ConditionsSet(&conditions, v1.Condition{
		Type:  v1.ConditionTypeRunning,
		State: v1.ConditionStateTrue,
	})
	require.Equal(t, []v1.Condition{
		{
			Type:  v1.ConditionTypeScheduled,
			State: v1.ConditionStateTrue,
		},
		{
			Type:  v1.ConditionTypeRunning,
			State: v1.ConditionStateTrue,
		},
	}, conditions)
}

func TestConditionsHumanize(t *testing.T) {
	conditions := []v1.Condition{
		{
			Type:  v1.ConditionTypeScheduled,
			State: v1.ConditionStateTrue,
		},
		{
			Type:  v1.ConditionTypeRunning,
			State: v1.ConditionStateFalse,
		},
	}

	require.Equal(t, "scheduled, not running", v1.ConditionsHumanize(conditions))

	conditions = []v1.Condition{
		{
			Type: v1.ConditionTypeScheduled,
		},
		{
			Type: v1.ConditionTypeRunning,
		},
	}

	require.Equal(t, "unknown scheduled, unknown running", v1.ConditionsHumanize(conditions))
}

func TestConditionMembershipChecks(t *testing.T) {
	// Condition does not exist
	var conditions []v1.Condition
	require.False(t, v1.ConditionExists(conditions, v1.ConditionTypeScheduled))
	require.False(t, v1.ConditionIsTrue(conditions, v1.ConditionTypeScheduled))
	require.False(t, v1.ConditionIsFalse(conditions, v1.ConditionTypeScheduled))

	// Condition exists, but its state is unknown
	conditions = []v1.Condition{
		{
			Type: v1.ConditionTypeScheduled,
		},
	}
	require.True(t, v1.ConditionExists(conditions, v1.ConditionTypeScheduled))
	require.False(t, v1.ConditionIsTrue(conditions, v1.ConditionTypeScheduled))
	require.False(t, v1.ConditionIsFalse(conditions, v1.ConditionTypeScheduled))

	// Condition exists and its state is true
	conditions = []v1.Condition{
		{
			Type:  v1.ConditionTypeScheduled,
			State: v1.ConditionStateTrue,
		},
	}
	require.True(t, v1.ConditionExists(conditions, v1.ConditionTypeScheduled))
	require.True(t, v1.ConditionIsTrue(conditions, v1.ConditionTypeScheduled))
	require.False(t, v1.ConditionIsFalse(conditions, v1.ConditionTypeScheduled))

	// Condition exists and its state is false
	conditions = []v1.Condition{
		{
			Type:  v1.ConditionTypeScheduled,
			State: v1.ConditionStateFalse,
		},
	}
	require.True(t, v1.ConditionExists(conditions, v1.ConditionTypeScheduled))
	require.False(t, v1.ConditionIsTrue(conditions, v1.ConditionTypeScheduled))
	require.True(t, v1.ConditionIsFalse(conditions, v1.ConditionTypeScheduled))
}
