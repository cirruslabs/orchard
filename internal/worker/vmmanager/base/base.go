package base

import (
	"sync/atomic"

	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	mapset "github.com/deckarep/golang-set/v2"
	"go.uber.org/zap"
)

type VM struct {
	// Backward compatibility with v1.VM specification's "Status" field
	//
	// "started" is always true after the first "tart run",
	// whereas ConditionReady can be used to tell if a VM
	// is really running or not.
	started atomic.Bool

	// A more orthogonal alternative to v1.VM specification's "Status" field,
	// which allows a VM to have more than one state.
	//
	// For example, a VM can be both in ConditionReady and ConditionSuspending/
	// ConditionStopping states for a short time. This way in run() we know
	// that we're in a process of rebooting a VM, so we can avoid throwing
	// an error about unexpected VM termination.
	conditions mapset.Set[v1.ConditionType]

	statusMessage atomic.Pointer[string]
	err           atomic.Pointer[error]

	logger *zap.SugaredLogger
}

func NewVM(logger *zap.SugaredLogger) *VM {
	return &VM{
		conditions: mapset.NewSet(v1.ConditionTypeCloning),
		logger:     logger,
	}
}

func (vm *VM) SetStarted(val bool) {
	vm.started.Store(val)
}

func (vm *VM) Status() v1.VMStatus {
	if vm.Err() != nil {
		return v1.VMStatusFailed
	}

	if vm.started.Load() {
		return v1.VMStatusRunning
	}

	return v1.VMStatusPending
}

func (vm *VM) StatusMessage() string {
	status := vm.statusMessage.Load()

	if status != nil {
		return *status
	}

	return ""
}

func (vm *VM) SetStatusMessage(status string) {
	vm.logger.Debugf(status)
	vm.statusMessage.Store(&status)
}

func (vm *VM) Err() error {
	if err := vm.err.Load(); err != nil {
		return *err
	}

	return nil
}

func (vm *VM) SetErr(err error) {
	if vm.err.CompareAndSwap(nil, &err) {
		vm.logger.Error(err)
	}
}

func (vm *VM) ConditionsSet() mapset.Set[v1.ConditionType] {
	return vm.conditions
}

func (vm *VM) Conditions() []v1.Condition {
	// Only expose a minimum amount of conditions necessary
	// for the Orchard Controller to make decisions
	return []v1.Condition{
		vm.conditionTypeToCondition(v1.ConditionTypeRunning),
	}
}

func (vm *VM) conditionTypeToCondition(conditionType v1.ConditionType) v1.Condition {
	var conditionState v1.ConditionState

	if vm.ConditionsSet().ContainsOne(conditionType) {
		conditionState = v1.ConditionStateTrue
	} else {
		conditionState = v1.ConditionStateFalse
	}

	return v1.Condition{
		Type:  conditionType,
		State: conditionState,
	}
}
