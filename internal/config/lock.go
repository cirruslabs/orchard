//go:build !windows

package config

import "github.com/gofrs/flock"

func (handle *Handle) Lock() (func(), error) {
	lock := flock.New(handle.configPath)

	if err := lock.Lock(); err != nil {
		return nil, err
	}

	return func() {
		_ = lock.Unlock()
	}, nil
}
