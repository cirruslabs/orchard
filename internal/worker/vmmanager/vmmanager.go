package vmmanager

import (
	vmpkg "github.com/cirruslabs/orchard/internal/worker/vm"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
)

type VMManager struct {
	vms map[string]*vmpkg.VM
}

func New() *VMManager {
	return &VMManager{
		vms: map[string]*vmpkg.VM{},
	}
}

func (vmm *VMManager) Exists(vmResource v1.VM) bool {
	_, ok := vmm.vms[vmResource.UID]

	return ok
}

func (vmm *VMManager) Get(vmResource v1.VM) (*vmpkg.VM, bool) {
	vm, ok := vmm.vms[vmResource.UID]

	return vm, ok
}

func (vmm *VMManager) Put(vm *vmpkg.VM) *vmpkg.VM {
	vmm.vms[vm.Resource.UID] = vm

	return vm
}

func (vmm *VMManager) Delete(vmResource v1.VM) {
	delete(vmm.vms, vmResource.UID)
}

func (vmm *VMManager) Len() int {
	return len(vmm.vms)
}

func (vmm *VMManager) List() []*vmpkg.VM {
	var vms []*vmpkg.VM

	for _, vm := range vmm.vms {
		vms = append(vms, vm)
	}

	return vms
}
