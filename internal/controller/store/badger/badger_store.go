package badger

import (
	"errors"
	"fmt"
	"github.com/avast/retry-go/v4"
	"github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/dgraph-io/badger/v3"
	"go.uber.org/zap"
	"time"
)

type Store struct {
	db *badger.DB
	store.Store
}

type Transaction struct {
	badgerTxn *badger.Txn
	store.Transaction
}

func NewBadgerStore(dbPath string, logger *zap.SugaredLogger) (store.Store, error) {
	opts := badger.DefaultOptions(dbPath).
		WithSyncWrites(true).
		WithNumVersionsToKeep(0).
		WithLogger(newBadgerLogger(logger))

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	store := &Store{
		db: db,
	}

	// Perform garbage collection periodically, as recommended in the documentation[1]
	//
	// [1]: https://docs.hypermode.com/badger/quickstart#garbage-collection
	go func() {
		for {
			if err := store.performGarbageCollection(); err != nil {
				logger.Errorf("garbage collection failed: %v", err)
			}

			<-time.After(1 * time.Hour)
		}
	}()

	return store, nil
}

func (store *Store) performGarbageCollection() error {
	// RunValueLogGC() needs to be invoked multiple times
	for {
		if err := store.db.RunValueLogGC(0.5); err != nil {
			// Nothing to rewrite, stop
			if errors.Is(err, badger.ErrNoRewrite) {
				return nil
			}

			return err
		}
	}
}

func (store *Store) View(cb func(txn store.Transaction) error) error {
	return retry.Do(func() error {
		return store.db.View(func(txn *badger.Txn) error {
			return cb(&Transaction{
				badgerTxn: txn,
			})
		})
	}, retry.RetryIf(func(err error) bool {
		return errors.Is(err, badger.ErrConflict)
	}), retry.Attempts(3), retry.LastErrorOnly(true))
}

func (store *Store) Update(cb func(txn store.Transaction) error) error {
	return retry.Do(func() error {
		return store.db.Update(func(txn *badger.Txn) error {
			return cb(&Transaction{
				badgerTxn: txn,
			})
		})
	}, retry.RetryIf(func(err error) bool {
		return errors.Is(err, badger.ErrConflict)
	}), retry.Attempts(3), retry.LastErrorOnly(true))
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
