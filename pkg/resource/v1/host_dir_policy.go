package v1

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidHostDirPolicy = errors.New("invalid hostDir policy")

type HostDirPolicy struct {
	PathPrefix string `json:"pathPrefix"`
	ReadOnly   bool   `json:"ro"`
}

func NewHostDirPolicyFromString(s string) (HostDirPolicy, error) {
	return HostDirPolicy{
		PathPrefix: strings.TrimSuffix(s, ":ro"),
		ReadOnly:   strings.HasSuffix(s, ":ro"),
	}, nil
}

func (policy HostDirPolicy) Validate(path string, readOnly bool) bool {
	if strings.Contains(path, "..") {
		return false
	}

	if policy.ReadOnly && !readOnly {
		return false
	}

	return strings.HasPrefix(
		strings.TrimSuffix(path, "/"),
		strings.TrimSuffix(policy.PathPrefix, "/"),
	)
}

func (policy HostDirPolicy) String() string {
	var roPart string

	if policy.ReadOnly {
		roPart = ":ro"
	}

	return fmt.Sprintf("%s%s", policy.PathPrefix, roPart)
}
