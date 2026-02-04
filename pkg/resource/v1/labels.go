package v1

import (
	"fmt"
	"strings"
)

type Labels map[string]string

func (labels Labels) Contains(other Labels) bool {
	for label, value := range other {
		if labels[label] != value {
			return false
		}
	}

	return true
}

func (labels Labels) String() string {
	var kvs []string

	for key, value := range labels {
		kvs = append(kvs, fmt.Sprintf("%s: %s", key, value))
	}

	return strings.Join(kvs, ", ")
}
