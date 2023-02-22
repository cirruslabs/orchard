package vmmanager

import (
	"errors"
	"fmt"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"go.uber.org/zap"
)

var ErrFailed = errors.New("VM manager failed")

type VMManager struct {
	vms map[string]*VM
}

func New() *VMManager {
	return &VMManager{
		vms: map[string]*VM{},
	}
}

func (vmm *VMManager) Exists(vmResource v1.VM) bool {
	_, ok := vmm.vms[vmResource.UID]

	return ok
}

func (vmm *VMManager) Get(vmResource v1.VM) (*VM, error) {
	managedVM, ok := vmm.vms[vmResource.UID]
	if !ok {
		return nil, fmt.Errorf("%w: VM does not exist", ErrFailed)
	}

	return managedVM, nil
}

func (vmm *VMManager) Create(vmResource v1.VM, logger *zap.SugaredLogger) (*VM, error) {
	if _, ok := vmm.vms[vmResource.UID]; ok {
		return nil, fmt.Errorf("%w: VM already exists", ErrFailed)
	}

	managedVM := NewVM(vmResource, logger)

	vmm.vms[vmResource.UID] = managedVM

	return managedVM, nil
}

func (vmm *VMManager) Stop(vmResource v1.VM) error {
	managedVM, ok := vmm.vms[vmResource.UID]
	if !ok {
		return fmt.Errorf("%w: VM does not exist", ErrFailed)
	}

	return managedVM.Stop()
}

func (vmm *VMManager) Delete(vmResource v1.VM) error {
	managedVM, ok := vmm.vms[vmResource.UID]
	if !ok {
		return fmt.Errorf("%w: VM does not exist", ErrFailed)
	}

	if err := managedVM.Delete(); err != nil {
		return err
	}

	delete(vmm.vms, vmResource.UID)

	return nil
}

func (vmm *VMManager) List() []*VM {
	var vms []*VM

	for _, vm := range vmm.vms {
		vms = append(vms, vm)
	}

	return vms
}
