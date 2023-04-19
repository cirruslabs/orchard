package vm

import (
	"context"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/cirruslabs/orchard/internal/worker/tart"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/qmuntal/stateless"
	"go.uber.org/zap"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrVMFailed = errors.New("VM failed")

type State string

const (
	StateInitial State = "initial"
	StateStopped State = "stopped"
	StateStarted State = "started"
	StateDeleted State = "deleted"
	StateFailed  State = "failed"
)

type Action string

const (
	ActionInit   Action = "init"
	ActionStart  Action = "start"
	ActionStop   Action = "stop"
	ActionDelete Action = "delete"
	ActionFail   Action = "fail"
)

type actionWithArgs struct {
	action Action
	args   []interface{}
}

type VM struct {
	id            string
	Resource      v1.VM
	eventStreamer *client.EventStreamer
	logger        *zap.SugaredLogger

	// Context that covers the whole lifecycle of this VM
	ctx       context.Context
	ctxCancel context.CancelFunc

	stateMachine     *stateless.StateMachine
	actionWithArgsCh chan actionWithArgs

	err    error
	errMtx sync.Mutex
}

func New(vmResource v1.VM, eventStreamer *client.EventStreamer, logger *zap.SugaredLogger) *VM {
	vm := &VM{
		id:            ondiskname.New(vmResource.Name, vmResource.UID).String(),
		Resource:      vmResource,
		eventStreamer: eventStreamer,
		logger:        logger.With("vm_name", vmResource.Name, "vm_uid", vmResource.UID),

		stateMachine:     stateless.NewStateMachine(StateInitial),
		actionWithArgsCh: make(chan actionWithArgs),
	}

	vm.ctx, vm.ctxCancel = context.WithCancel(context.Background())

	vm.stateMachine.Configure(StateInitial).
		Permit(ActionInit, StateStopped).
		Permit(ActionFail, StateFailed).
		OnExit(func(ctx context.Context, args ...interface{}) error {
			if err := vm.cloneAndConfigure(ctx); err != nil {
				return vm.stateMachine.FireCtx(ctx, ActionFail, err)
			}

			return nil
		})

	vm.stateMachine.Configure(StateStopped).
		Permit(ActionStart, StateStarted).
		Permit(ActionDelete, StateDeleted)

	vm.stateMachine.Configure(StateStarted).
		Permit(ActionStop, StateStopped).
		Permit(ActionDelete, StateDeleted).
		Permit(ActionFail, StateFailed).
		OnEntry(func(ctx context.Context, args ...interface{}) error {
			vm.start()

			return nil
		}).
		OnExit(func(ctx context.Context, args ...interface{}) error {
			if err := vm.stop(ctx); err != nil {
				return vm.stateMachine.FireCtx(ctx, ActionFail, err)
			}

			return nil
		})

	vm.stateMachine.Configure(StateDeleted).
		OnEntry(func(ctx context.Context, args ...interface{}) error {
			if err := vm.delete(ctx); err != nil {
				return vm.stateMachine.FireCtx(ctx, ActionFail, err)
			}

			return nil
		})

	vm.stateMachine.Configure(StateFailed).
		Permit(ActionDelete, StateDeleted).
		Ignore(ActionFail).
		OnEntry(func(ctx context.Context, args ...interface{}) error {
			vm.setErr(args[0].(error))

			return nil
		})

	// Run the state machine in background
	go func() {
		for {
			select {
			case actionWithArgs := <-vm.actionWithArgsCh:
				if err := vm.stateMachine.Fire(actionWithArgs.action, actionWithArgs.args...); err != nil {
					vm.logger.Error(err)
				}
			case <-vm.ctx.Done():
				return
			}
		}
	}()

	vm.Action(ActionInit)

	return vm
}

func (vm *VM) State() State {
	return vm.stateMachine.MustState().(State)
}

func (vm *VM) Action(action Action, args ...interface{}) {
	vm.actionWithArgsCh <- actionWithArgs{
		action: action,
		args:   args,
	}
}

func (vm *VM) IP(ctx context.Context) (string, error) {
	stdout, _, err := tart.Tart(ctx, vm.logger, "ip", "--wait", "60", vm.id)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (vm *VM) Close() error {
	vm.Action(ActionDelete)

	for {
		if vm.State() == StateDeleted {
			break
		}

		time.Sleep(time.Second)
	}

	vm.ctxCancel()

	if vm.eventStreamer != nil {
		return vm.eventStreamer.Close()
	}

	return nil
}

func (vm *VM) Err() error {
	vm.errMtx.Lock()
	defer vm.errMtx.Unlock()

	return vm.err
}

func (vm *VM) setErr(err error) {
	vm.errMtx.Lock()
	defer vm.errMtx.Unlock()

	if vm.err == nil {
		vm.err = err
	}
}

func (vm *VM) cloneAndConfigure(ctx context.Context) error {
	_, _, err := tart.Tart(ctx, vm.logger, "clone", vm.Resource.Image, vm.id)
	if err != nil {
		return fmt.Errorf("%w: failed to clone VM: %v", ErrVMFailed, err)
	}

	if vm.Resource.Memory != 0 {
		_, _, err = tart.Tart(ctx, vm.logger, "set", "--memory",
			strconv.FormatUint(vm.Resource.Memory, 10), vm.id)
		if err != nil {
			return fmt.Errorf("%w: failed to configure VM: %v", ErrVMFailed, err)
		}
	}

	if vm.Resource.CPU != 0 {
		_, _, err = tart.Tart(ctx, vm.logger, "set", "--cpu",
			strconv.FormatUint(vm.Resource.CPU, 10), vm.id)
		if err != nil {
			return fmt.Errorf("%w: failed to configure VM: %v", ErrVMFailed, err)
		}
	}
	return nil
}

func (vm *VM) start() {
	vm.logger.Debugf("starting VM")

	// Context that only covers a single run of this VM
	runCtx, runCtxCancel := context.WithCancel(vm.ctx)

	if vm.Resource.StartupScript != nil {
		go func() {
			vm.logger.Debugf("running startup script")

			if err := vm.shellHelper(runCtx, vm.Resource, vm.Resource.StartupScript); err != nil {
				err := fmt.Errorf("%w: failed to run startup script: %v", ErrVMFailed, err)
				vm.logger.Error(err)
				vm.Action(ActionFail, err)
				runCtxCancel()
			}
		}()
	}

	go func() {
		defer runCtxCancel()

		if err := vm.run(runCtx); err != nil {
			err := fmt.Errorf("%w: failed to run the VM: %v", ErrVMFailed, err)
			vm.logger.Error(err)
			vm.Action(ActionFail, err)
		}
	}()
}

func (vm *VM) run(ctx context.Context) error {
	var runArgs = []string{"run"}

	if vm.Resource.Softnet {
		runArgs = append(runArgs, "--net-softnet")
	}

	if vm.Resource.Headless {
		runArgs = append(runArgs, "--no-graphics")
	}

	runArgs = append(runArgs, vm.id)
	_, _, err := tart.Tart(ctx, vm.logger, runArgs...)
	if err != nil {
		return err
	}

	return nil
}

func (vm *VM) stop(ctx context.Context) error {
	vm.logger.Debugf("stopping VM")

	if vm.Resource.ShutdownScript != nil {
		vm.logger.Debugf("running shutdown script")

		if err := vm.shellHelper(ctx, vm.Resource, vm.Resource.ShutdownScript); err != nil {
			err := fmt.Errorf("%w: failed to run shutdown script: %v", ErrVMFailed, err)
			vm.logger.Error(err)
		}
	}

	_, _, err := tart.Tart(ctx, vm.logger, "stop", vm.id)
	if err != nil {
		return fmt.Errorf("%w: failed to stop the VM: %v", ErrVMFailed, err)
	}

	vm.logger.Debugf("stopped VM")

	return nil
}

func (vm *VM) delete(ctx context.Context) error {
	vm.logger.Debugf("deleting VM")

	_, _, err := tart.Tart(ctx, vm.logger, "delete", vm.id)
	if err != nil {
		return fmt.Errorf("%w: failed to delete the VM: %v", ErrVMFailed, err)
	}

	return nil
}
