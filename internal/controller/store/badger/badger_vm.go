//nolint:dupl // maybe we'll figure out how to make DB resource accessors generic in the future
package badger

import (
	"path"

	"github.com/cirruslabs/orchard/pkg/resource/v1"
)

const SpaceVMs = "/vms"

func VMKey(name string) []byte {
	return []byte(path.Join(SpaceVMs, name))
}

func (txn *Transaction) GetVM(name string) (*v1.VM, error) {
	return genericGet[v1.VM](txn, VMKey(name))
}

func (txn *Transaction) SetVM(vm v1.VM) error {
	return genericSet[v1.VM](txn, VMKey(vm.Name), vm)
}

func (txn *Transaction) DeleteVM(name string) error {
	return genericDelete(txn, VMKey(name))
}

func (txn *Transaction) ListVMs() ([]v1.VM, error) {
	return genericList[v1.VM](txn, []byte(SpaceVMs))
}
