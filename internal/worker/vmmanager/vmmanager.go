package vmmanager

import (
	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
)

type VMManager struct {
	vms map[ondiskname.OnDiskName]VM
}

func New() *VMManager {
	return &VMManager{
		vms: map[ondiskname.OnDiskName]VM{},
	}
}

func (vmm *VMManager) Exists(key ondiskname.OnDiskName) bool {
	_, ok := vmm.vms[key]

	return ok
}

func (vmm *VMManager) Get(key ondiskname.OnDiskName) (VM, bool) {
	vm, ok := vmm.vms[key]

	return vm, ok
}

func (vmm *VMManager) Put(key ondiskname.OnDiskName, vm VM) {
	vmm.vms[key] = vm
}

func (vmm *VMManager) Delete(key ondiskname.OnDiskName) {
	delete(vmm.vms, key)
}

func (vmm *VMManager) Len() int {
	return len(vmm.vms)
}

func (vmm *VMManager) List() []VM {
	var vms []VM

	for _, vm := range vmm.vms {
		vms = append(vms, vm)
	}

	return vms
}
