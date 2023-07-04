package v1

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidHostDir = errors.New("invalid hostDir specification")

type HostDir struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	ReadOnly bool   `json:"ro"`
}

func NewHostDirFromString(s string) (HostDir, error) {
	parts := strings.Split(s, ":")

	if len(parts) > 3 {
		return HostDir{}, fmt.Errorf("%w: hostDir specification can only contain 3 parts at max",
			ErrInvalidHostDir)
	}

	if parts[0] == "" {
		return HostDir{}, fmt.Errorf("%w: name cannot be empty", ErrInvalidHostDir)
	}
	if parts[1] == "" {
		return HostDir{}, fmt.Errorf("%w: path cannot be empty", ErrInvalidHostDir)
	}

	var readOnly bool

	if len(parts) == 3 {
		if parts[2] == "ro" {
			readOnly = true
		} else {
			return HostDir{}, fmt.Errorf("%w: hostDir's third part can only be \"ro\", got %q",
				ErrInvalidHostDir, parts[2])
		}
	}

	return HostDir{
		Name:     parts[0],
		Path:     parts[1],
		ReadOnly: readOnly,
	}, nil
}

func (hostDir HostDir) String() string {
	var roPart string

	if hostDir.ReadOnly {
		roPart = ":ro"
	}

	return fmt.Sprintf("%s:%s%s", hostDir.Name, hostDir.Path, roPart)
}
