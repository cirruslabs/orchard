package v1

import (
	"fmt"
	"strings"
)

type Condition struct {
	Type  ConditionType  `json:"type"`
	State ConditionState `json:"state"`
}

type ConditionType string

const (
	// VM conditions
	ConditionTypeScheduled ConditionType = "scheduled"
	ConditionTypeRunning   ConditionType = "running"

	// VM conditions (internal to worker)
	ConditionTypeCloning    ConditionType = "cloning"
	ConditionTypeSuspending ConditionType = "suspending"
	ConditionTypeStopping   ConditionType = "stopping"

	// ImagePull and ImagePullJob conditions
	ConditionTypeProgressing ConditionType = "progressing"
	ConditionTypeCompleted   ConditionType = "completed"
	ConditionTypeFailed      ConditionType = "failed"
)

type ConditionState string

const (
	ConditionStateTrue  ConditionState = "true"
	ConditionStateFalse ConditionState = "false"
)

func ConditionsSet(conditions *[]Condition, newCondition Condition) bool {
	for i := range *conditions {
		condition := &(*conditions)[i]

		if condition.Type != newCondition.Type {
			continue
		}

		if condition.State == newCondition.State {
			return false
		}

		condition.State = newCondition.State

		return true
	}

	*conditions = append(*conditions, newCondition)

	return true
}

func ConditionsHumanize(conditions []Condition) string {
	var conditionHumanized []string

	for _, condition := range conditions {
		var pre string

		switch condition.State {
		case ConditionStateTrue:
			// Nothing needs to be set
		case ConditionStateFalse:
			pre = "not "
		default:
			pre = "unknown "
		}

		conditionHumanized = append(conditionHumanized, fmt.Sprintf("%s%s", pre, condition.Type))
	}

	return strings.Join(conditionHumanized, ", ")
}

func ConditionExists(conditions []Condition, conditionType ConditionType) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return true
		}
	}

	return false
}

func ConditionIsTrue(conditions []Condition, conditionType ConditionType) bool {
	return conditionIsTypeAndState(conditions, conditionType, ConditionStateTrue)
}

func ConditionIsFalse(conditions []Condition, conditionType ConditionType) bool {
	return conditionIsTypeAndState(conditions, conditionType, ConditionStateFalse)
}

func conditionIsTypeAndState(conditions []Condition, conditionType ConditionType, state ConditionState) bool {
	for _, condition := range conditions {
		if condition.Type != conditionType {
			continue
		}

		if condition.State != state {
			return false
		}

		return true
	}

	return false
}
