package vmmanager

import (
	"errors"
	"fmt"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
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

func (vmm *VMManager) Exists(vmResource *v1.VM) bool {
	_, ok := vmm.vms[vmResource.UID]

	return ok
}

func (vmm *VMManager) Get(vmResource *v1.VM) (*VM, error) {
	managedVM, ok := vmm.vms[vmResource.UID]
	if !ok {
		return nil, fmt.Errorf("%w: VM does not exist", ErrFailed)
	}

	return managedVM, nil
}

func (vmm *VMManager) Create(vmResource *v1.VM) (*VM, error) {
	if _, ok := vmm.vms[vmResource.UID]; ok {
		return nil, fmt.Errorf("%w: VM already exists", ErrFailed)
	}

	managedVM := NewVM(vmResource)

	vmm.vms[vmResource.UID] = managedVM

	return managedVM, nil
}

func (vmm *VMManager) Delete(vmResource *v1.VM) error {
	managedVM, ok := vmm.vms[vmResource.UID]
	if !ok {
		return fmt.Errorf("%w: VM does not exist", ErrFailed)
	}

	if err := managedVM.Close(); err != nil {
		return err
	}

	delete(vmm.vms, vmResource.UID)

	return nil
}
