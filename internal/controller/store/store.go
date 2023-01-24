package store

import (
	"github.com/dgraph-io/badger/v3"
)

type Store struct {
	db *badger.DB
}

func New(dbPath string) (*Store, error) {
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
