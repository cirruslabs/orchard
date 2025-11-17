//nolint:dupl // maybe we'll figure out how to make DB resource accessors generic in the future
package badger

import (
	"path"

	"github.com/cirruslabs/orchard/pkg/resource/v1"
)

const SpaceWorkers = "/workers"

func WorkerKey(name string) []byte {
	return []byte(path.Join(SpaceWorkers, name))
}

func (txn *Transaction) GetWorker(name string) (*v1.Worker, error) {
	return genericGet[v1.Worker](txn, WorkerKey(name))
}

func (txn *Transaction) SetWorker(worker v1.Worker) error {
	return genericSet[v1.Worker](txn, WorkerKey(worker.Name), worker)
}

func (txn *Transaction) DeleteWorker(name string) error {
	return genericDelete(txn, WorkerKey(name))
}

func (txn *Transaction) ListWorkers() ([]v1.Worker, error) {
	return genericList[v1.Worker](txn, []byte(SpaceWorkers))
}
