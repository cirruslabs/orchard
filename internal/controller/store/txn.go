package store

import "github.com/dgraph-io/badger/v3"

type Txn struct {
	badgerTxn *badger.Txn
}

func (store *Store) View(cb func(txn *Txn) error) error {
	return store.db.View(func(txn *badger.Txn) error {
		return cb(&Txn{
			badgerTxn: txn,
		})
	})
}

func (store *Store) Update(cb func(txn *Txn) error) error {
	return store.db.Update(func(txn *badger.Txn) error {
		return cb(&Txn{
			badgerTxn: txn,
		})
	})
}
