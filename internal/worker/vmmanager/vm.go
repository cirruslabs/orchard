package vmmanager

import (
	"context"
	"fmt"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"go.uber.org/zap"
	"strconv"
	"strings"
	"sync"
)

type VM struct {
	id       string
	Resource v1.VM

	ctx    context.Context
	cancel context.CancelFunc

	wg *sync.WaitGroup
}

func NewVM(vmResource v1.VM, logger *zap.SugaredLogger) *VM {
	ctx, cancel := context.WithCancel(context.Background())

	vm := &VM{
		id:       fmt.Sprintf("orchard-%s-%s", vmResource.Name, vmResource.UID),
		Resource: vmResource,

		ctx:    ctx,
		cancel: cancel,

		wg: &sync.WaitGroup{},
	}

	vm.wg.Add(1)

	go func() {
		defer vm.wg.Done()

		// Optimistic set the status to running. Will be synced later by the worker loop.
		vm.Resource.Status = v1.VMStatusRunning
		if err := vm.run(vm.ctx); err != nil {
			logger.Errorf("VM %s failed: %v", vm.id, err)
			vm.Resource.Status = v1.VMStatusFailed
		} else {
			vm.Resource.Status = v1.VMStatusStopped
		}
	}()

	return vm
}

func (vm *VM) run(ctx context.Context) error {
	_, _, err := Tart(ctx, "clone", vm.Resource.Image, vm.id)
	if err != nil {
		return err
	}

	if vm.Resource.Memory != 0 {
		_, _, err = Tart(ctx, "set", "--memory", strconv.FormatUint(vm.Resource.Memory, 10), vm.id)
		if err != nil {
			return err
		}
	}

	if vm.Resource.CPU != 0 {
		_, _, err = Tart(ctx, "set", "--cpu", strconv.FormatUint(vm.Resource.CPU, 10), vm.id)
		if err != nil {
			return err
		}
	}

	var runArgs = []string{"run"}

	if vm.Resource.Softnet {
		runArgs = append(runArgs, "--net-softnet")
	}

	if vm.Resource.Headless {
		runArgs = append(runArgs, "--no-graphics")
	}

	runArgs = append(runArgs, vm.id)
	_, _, err = Tart(ctx, runArgs...)
	if err != nil {
		return err
	}

	return nil
}

func (vm *VM) IP(ctx context.Context) (string, error) {
	stdout, _, err := Tart(ctx, "ip", vm.id)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (vm *VM) Stop() error {
	_, _, _ = Tart(context.Background(), "stop", vm.id)

	vm.cancel()

	vm.wg.Wait()

	return nil
}

func (vm *VM) Delete() error {
	_, _, err := Tart(context.Background(), "delete", vm.id)
	if err != nil {
		return fmt.Errorf("%w: failed to delete VM %s: %v", ErrFailed, vm.id, err)
	}

	return nil
}
