package config

import "errors"

var (
	ErrConfigReadFailed  = errors.New("failed to read configuration file")
	ErrConfigWriteFailed = errors.New("failed to write configuration file")
	ErrConfigConflict    = errors.New("conflict while operating on configuration file")
)
