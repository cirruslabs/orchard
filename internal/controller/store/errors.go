package store

import (
	"errors"
	"fmt"
	"github.com/dgraph-io/badger/v3"
)

var (
	ErrNotFound     = errors.New("DB entry not found")
	ErrBadgerFailed = errors.New("BadgerDB failed")
)

func mapErr(err error) error {
	if errors.Is(err, badger.ErrKeyNotFound) {
		return ErrNotFound
	}

	return fmt.Errorf("%w: %v", ErrBadgerFailed, err)
}
