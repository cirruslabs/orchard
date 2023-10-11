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
	var readOnly bool

	// Detect read-only (":ro") modifier
	// and remove it from the string
	if strings.HasSuffix(s, ":ro") {
		s = strings.TrimSuffix(s, ":ro")
		readOnly = true
	}

	// Limit the maximum number of splits to 2
	// to support "http{,s}://..." paths[1]
	//                     ^
	// [1]: https://github.com/cirruslabs/tart/pull/620
	parts := strings.SplitN(s, ":", 2)

	if len(parts) < 2 {
		return HostDir{}, fmt.Errorf("%w: hostDir specification needs to contain a name and a path "+
			"separated by a colon (\":\")", ErrInvalidHostDir)
	}

	if parts[0] == "" {
		return HostDir{}, fmt.Errorf("%w: name cannot be empty", ErrInvalidHostDir)
	}
	if parts[1] == "" {
		return HostDir{}, fmt.Errorf("%w: path cannot be empty", ErrInvalidHostDir)
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
