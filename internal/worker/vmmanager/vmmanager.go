package vmmanager

import (
	"context"

	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
)

type VM interface {
	Resource() v1.VM
	SetResource(vmResource v1.VM)
	OnDiskName() ondiskname.OnDiskName
	ImageFQN() *string
	Status() v1.VMStatus
	StatusMessage() string
	Err() error
	Conditions() []v1.Condition

	Start(eventStreamer *client.EventStreamer)
	Suspend() <-chan error
	IP(ctx context.Context) (string, error)
	Stop() <-chan error
	Delete() error
}

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
