package badger

import (
	"encoding/json"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/dgraph-io/badger/v3"
	"path"
)

const SpaceServiceAccounts = "/service-accounts"

func ServiceAccountKey(name string) []byte {
	return []byte(path.Join(SpaceServiceAccounts, name))
}

func (txn *Transaction) GetServiceAccount(name string) (_ *v1.ServiceAccount, err error) {
	defer func() {
		err = mapErr(err)
	}()

	key := ServiceAccountKey(name)

	item, err := txn.badgerTxn.Get(key)
	if err != nil {
		return nil, err
	}

	valueBytes, err := item.ValueCopy(nil)
	if err != nil {
		return nil, err
	}

	var serviceAccount v1.ServiceAccount

	err = json.Unmarshal(valueBytes, &serviceAccount)
	if err != nil {
		return nil, err
	}

	return &serviceAccount, nil
}

func (txn *Transaction) SetServiceAccount(serviceAccount *v1.ServiceAccount) (err error) {
	defer func() {
		err = mapErr(err)
	}()

	key := ServiceAccountKey(serviceAccount.Name)

	valueBytes, err := json.Marshal(serviceAccount)
	if err != nil {
		return err
	}

	return txn.badgerTxn.Set(key, valueBytes)
}

func (txn *Transaction) DeleteServiceAccount(name string) (err error) {
	defer func() {
		err = mapErr(err)
	}()

	key := ServiceAccountKey(name)

	return txn.badgerTxn.Delete(key)
}

func (txn *Transaction) ListServiceAccounts() (_ []*v1.ServiceAccount, err error) {
	defer func() {
		err = mapErr(err)
	}()

	// Declare an empty, non-nil slice to return
	// [] when no service accounts are found
	result := []*v1.ServiceAccount{}

	it := txn.badgerTxn.NewIterator(badger.IteratorOptions{
		Prefix: []byte(SpaceServiceAccounts),
	})
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()

		serviceAccountBytes, err := item.ValueCopy(nil)
		if err != nil {
			return nil, err
		}

		var serviceAccount v1.ServiceAccount

		if err := json.Unmarshal(serviceAccountBytes, &serviceAccount); err != nil {
			return nil, err
		}

		result = append(result, &serviceAccount)
	}

	return result, nil
}
