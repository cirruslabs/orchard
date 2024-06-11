package simplename

import (
	"errors"
	"fmt"
)

var (
	ErrNotASimpleName = errors.New("name contains restricted characters, please only use [A-Za-z0-9:-_.]")

	ErrNotASimpleNameNext = errors.New("names with characters other than [a-z0-9-] will be deprecated in the future")
)

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

func ValidateNext(s string) error {
	// Ensure that the name is not empty
	if s == "" {
		return fmt.Errorf("name cannot be empty")
	}

	// Ensure that the name is 63 characters or fewer
	if len(s) > 63 {
		return fmt.Errorf("names with more than 63 characters " +
			"will be depreacted in the future")
	}

	// Ensure that the name starts and ends with an alphanumeric character
	if !isAlphanumeric(s[0]) {
		return fmt.Errorf("names not starting with an alphanumeric character " +
			"will be deprecated in the future")
	}

	if !isAlphanumeric(s[len(s)-1]) {
		return fmt.Errorf("names not ending with an alphanumeric character " +
			"will be deprecated in the future")
	}

	for i := range s {
		if isAlphanumeric(s[i]) {
			continue
		}

		if s[i] == '-' {
			continue
		}

		return ErrNotASimpleNameNext
	}

	return nil
}

func isAlphanumeric(ch uint8) bool {
	if ch >= 'a' && ch <= 'z' {
		return true
	}

	if ch >= '0' && ch <= '9' {
		return true
	}

	return false
}
