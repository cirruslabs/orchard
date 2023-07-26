package v1

import (
	"errors"
	"fmt"
)

var ErrInvalidImagePullPolicy = errors.New("invalid image pull policy")

type ImagePullPolicy string

const (
	ImagePullPolicyIfNotPresent ImagePullPolicy = "IfNotPresent"
	ImagePullPolicyAlways       ImagePullPolicy = "Always"
)

func NewImagePullPolicyFromString(s string) (ImagePullPolicy, error) {
	switch s {
	case string(ImagePullPolicyIfNotPresent):
		return ImagePullPolicyIfNotPresent, nil
	case string(ImagePullPolicyAlways):
		return ImagePullPolicyAlways, nil
	default:
		return "", fmt.Errorf("%w %q", ErrInvalidImagePullPolicy, s)
	}
}
