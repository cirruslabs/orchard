package store

import (
	"errors"
)

var (
	ErrNotFound    = errors.New("store entry not found")
	ErrConflict    = errors.New("store conflict")
	ErrStoreFailed = errors.New("store failed")
)
