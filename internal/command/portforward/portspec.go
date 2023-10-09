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

	switch len(splits) {
	case 1:
		remotePort, err := parsePort(splits[0])
		if err != nil {
			return nil, err
		}

		return &PortSpec{
			LocalPort:  0,
			RemotePort: remotePort,
		}, nil
	case 2:
		localPort, err := parsePort(splits[0])
		if err != nil {
			return nil, err
		}

		remotePort, err := parsePort(splits[1])
		if err != nil {
			return nil, err
		}

		return &PortSpec{
			LocalPort:  localPort,
			RemotePort: remotePort,
		}, nil
	default:
		return nil, fmt.Errorf("%w: expected 1 or 2 components delimited by \":\", found %d",
			ErrInvalidPortSpec, len(splits))
	}
}

func parsePort(s string) (uint16, error) {
	port, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, err
	}

	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("%w: only ports in range [1, 65535] are allowed", ErrInvalidPortSpec)
	}

	return uint16(port), nil
}
