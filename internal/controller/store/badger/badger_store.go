package badger

import (
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/dgraph-io/badger/v3"
)

type Store struct {
	db *badger.DB
	store.Store
}

type Transaction struct {
	badgerTxn *badger.Txn
	store.Transaction
}

func NewBadgerStore(dbPath string) (store.Store, error) {
	opts := badger.DefaultOptions(dbPath)

	opts.SyncWrites = true

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	return &Store{
		db: db,
	}, nil
}

func (store *Store) View(cb func(txn store.Transaction) error) error {
	return store.db.View(func(txn *badger.Txn) error {
		return cb(&Transaction{
			badgerTxn: txn,
		})
	})
}

func (store *Store) Update(cb func(txn store.Transaction) error) error {
	return store.db.Update(func(txn *badger.Txn) error {
		return cb(&Transaction{
			badgerTxn: txn,
		})
	})
}

func mapErr(err error) error {
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return store.ErrNotFound
		}

		return fmt.Errorf("%w: %v", store.ErrStoreFailed, err)
	}

	return err
}
