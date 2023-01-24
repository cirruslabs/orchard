package store

import (
	"encoding/json"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/dgraph-io/badger/v3"
	"path"
)

const SpaceWorkers = "/workers"

func WorkerKey(name string) []byte {
	return []byte(path.Join(SpaceWorkers, name))
}

func (txn *Txn) GetWorker(name string) (result *v1.Worker, err error) {
	defer func() {
		err = mapErr(err)
	}()

	key := WorkerKey(name)

	item, err := txn.badgerTxn.Get(key)
	if err != nil {
		return nil, err
	}

	valueBytes, err := item.ValueCopy(nil)
	if err != nil {
		return nil, err
	}

	var worker v1.Worker

	err = json.Unmarshal(valueBytes, &worker)
	if err != nil {
		return nil, err
	}

	return &worker, nil
}

func (txn *Txn) SetWorker(worker *v1.Worker) (err error) {
	defer func() {
		err = mapErr(err)
	}()

	key := WorkerKey(worker.Name)

	valueBytes, err := json.Marshal(worker)
	if err != nil {
		return err
	}

	return txn.badgerTxn.Set(key, valueBytes)
}

func (txn *Txn) DeleteWorker(name string) (err error) {
	defer func() {
		err = mapErr(err)
	}()

	key := WorkerKey(name)

	return txn.badgerTxn.Delete(key)
}

func (txn *Txn) ListWorkers() (result []*v1.Worker, err error) {
	defer func() {
		err = mapErr(err)
	}()

	it := txn.badgerTxn.NewIterator(badger.IteratorOptions{
		Prefix: []byte(SpaceWorkers),
	})
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()

		vmBytes, err := item.ValueCopy(nil)
		if err != nil {
			return nil, err
		}

		var worker v1.Worker

		if err := json.Unmarshal(vmBytes, &worker); err != nil {
			return nil, err
		}

		result = append(result, &worker)
	}

	return result, nil
}
