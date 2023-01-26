package vmmanager

import (
	"context"
	"fmt"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"sync"
)

type VM struct {
	id         string
	vmResource *v1.VM

	ctx    context.Context
	cancel context.CancelFunc

	wg *sync.WaitGroup
}

func NewVM(vmResource *v1.VM) *VM {
	ctx, cancel := context.WithCancel(context.Background())

	vm := &VM{
		id:         fmt.Sprintf("orchard-%s-%s", vmResource.Name, vmResource.UID),
		vmResource: vmResource,

		ctx:    ctx,
		cancel: cancel,

		wg: &sync.WaitGroup{},
	}

	vm.wg.Add(1)

	go func() {
		defer vm.wg.Done()

		if err := vm.run(vm.ctx); err != nil {
			vmResource.Status = v1.VMStatusFailed
		}
	}()

	return vm
}

func (vm *VM) run(ctx context.Context) error {
	_, _, err := Tart(ctx, "clone", vm.vmResource.Image, vm.id)
	if err != nil {
		return err
	}

	_, _, err = Tart(ctx, "run", vm.id)
	if err != nil {
		return err
	}

	return nil
}

func (vm *VM) Close() error {
	_, _, _ = Tart(context.Background(), "stop", "--timeout", "5", vm.id)

	vm.cancel()

	vm.wg.Wait()

	_, _, err := Tart(context.Background(), "delete", vm.id)
	if err != nil {
		return fmt.Errorf("failed to delete VM %s: %v", vm.id, err)
	}

	return nil
}
