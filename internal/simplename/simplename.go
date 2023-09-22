package simplename

import (
	"errors"
)

var ErrNotASimpleName = errors.New("name contains restricted characters, please only use [A-Za-z0-9:-_.]")

func Validate(s string) error {
	for _, ch := range s {
		if ch >= 'a' && ch <= 'z' {
			continue
		}

		if ch >= 'A' && ch <= 'Z' {
			continue
		}

		if ch >= '0' && ch <= '9' {
			continue
		}

		if ch == ':' || ch == '-' || ch == '_' || ch == '.' {
			continue
		}

		return ErrNotASimpleName
	}

	return nil
}
