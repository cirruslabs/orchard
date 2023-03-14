package portforward

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidPortSpec = errors.New("invalid port-forwarding specification")

type PortSpec struct {
	LocalPort  uint16
	RemotePort uint16
}

func NewPortSpec(portSpecRaw string) (*PortSpec, error) {
	splits := strings.Split(portSpecRaw, ":")

	if len(splits) > 2 {
		return nil, fmt.Errorf("%w: expected no more than 2 components delimited by \":\", found %d",
			ErrInvalidPortSpec, len(splits))
	}

	localPort, err := strconv.ParseUint(splits[0], 10, 16)
	if err != nil {
		return nil, err
	}

	remotePort := localPort

	if len(splits) > 1 {
		remotePort, err = strconv.ParseUint(splits[1], 10, 16)
		if err != nil {
			return nil, err
		}
	}

	if localPort < 1 || remotePort < 1 || localPort > 65535 || remotePort > 65535 {
		return nil, fmt.Errorf("%w: only ports in range [1, 65535] are allowed", ErrInvalidPortSpec)
	}

	return &PortSpec{
		LocalPort:  uint16(localPort),
		RemotePort: uint16(remotePort),
	}, nil
}
