package vmmanager

type Condition string

const (
	ConditionCloning    Condition = "cloning"
	ConditionReady      Condition = "ready"
	ConditionSuspending Condition = "suspending"
	ConditionStopping   Condition = "stopping"
)
