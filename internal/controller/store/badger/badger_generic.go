package badger

import (
	"encoding/json"

	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/dgraph-io/badger/v3"
)

func genericSet[T any](txn *Transaction, key []byte, obj T) (err error) {
	defer func() {
		err = mapErr(err)
	}()

	valueBytes, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	return txn.badgerTxn.Set(key, valueBytes)
}

func genericGet[T any, PT interface {
	SetVersion(uint64)
	*T
}](txn *Transaction, key []byte) (_ *T, err error) {
	defer func() {
		err = mapErr(err)
	}()

	item, err := txn.badgerTxn.Get(key)
	if err != nil {
		return nil, err
	}

	valueBytes, err := item.ValueCopy(nil)
	if err != nil {
		return nil, err
	}

	var obj T

	err = json.Unmarshal(valueBytes, &obj)
	if err != nil {
		return nil, err
	}

	PT(&obj).SetVersion(item.Version())

	return &obj, nil
}

func genericList[T any, PT interface {
	SetVersion(uint64)
	Match(v1.Filter) bool
	*T
}](txn *Transaction, prefix []byte, opts ...storepkg.ListOption) (_ []T, err error) {
	defer func() {
		err = mapErr(err)
	}()

	// Apply options
	listInput := &storepkg.ListInput{}

	for _, opt := range opts {
		opt(listInput)
	}

	// Declare an empty, non-nil slice to
	// return [] when no objects are found
	result := []T{}

	it := txn.badgerTxn.NewIterator(badger.IteratorOptions{
		Prefix: prefix,
	})
	defer it.Close()

Outer:
	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()

		valueBytes, err := item.ValueCopy(nil)
		if err != nil {
			return nil, err
		}

		var obj T

		if err := json.Unmarshal(valueBytes, &obj); err != nil {
			return nil, err
		}

		for _, filter := range listInput.Filters {
			if !PT(&obj).Match(filter) {
				continue Outer
			}
		}

		PT(&obj).SetVersion(item.Version())

		result = append(result, obj)
	}

	return result, nil
}

func genericDelete(txn *Transaction, key []byte) (err error) {
	defer func() {
		err = mapErr(err)
	}()

	return txn.badgerTxn.Delete(key)
}
