package v1

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidFilter = errors.New("invalid filter")

type Filter struct {
	Path  string
	Value string
}

func NewFilter(s string) (Filter, error) {
	parts := strings.SplitN(s, "=", 2)

	if len(parts) != 2 {
		return Filter{}, fmt.Errorf("%w: expected path=value", ErrInvalidFilter)
	}

	if parts[0] == "" {
		return Filter{}, fmt.Errorf("%w: path cannot be empty", ErrInvalidFilter)
	}

	return Filter{
		Path:  parts[0],
		Value: parts[1],
	}, nil
}
