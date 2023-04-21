package v1

import (
	"errors"
	"fmt"
)

var ErrInvalidRestartPolicy = errors.New("invalid restart policy")

type RestartPolicy string

const (
	RestartPolicyNever     RestartPolicy = "Never"
	RestartPolicyOnFailure RestartPolicy = "OnFailure"
)

func NewRestartPolicyFromString(s string) (RestartPolicy, error) {
	switch s {
	case string(RestartPolicyNever):
		return RestartPolicyNever, nil
	case string(RestartPolicyOnFailure):
		return RestartPolicyOnFailure, nil
	default:
		return "", fmt.Errorf("%w %q", ErrInvalidRestartPolicy, s)
	}
}
