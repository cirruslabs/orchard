package config

import (
	"github.com/gofrs/flock"
	"os"
)

func (handle *Handle) Lock() (func(), error) {
	lockPath := handle.configPath + ".lock"

	lock := flock.New(lockPath)

	if err := lock.Lock(); err != nil {
		return nil, err
	}

	return func() {
		_ = lock.Unlock()
		_ = os.Remove(lockPath)
	}, nil
}
