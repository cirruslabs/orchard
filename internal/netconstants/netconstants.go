package netconstants

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	DefaultControllerPort       = 6120
	DefaultControllerServerName = "orchard-controller"
)

func NormalizeAddress(addr string) (*url.URL, error) {
	if !strings.HasPrefix(addr, "https://") && !strings.HasPrefix(addr, "http://") {
		addr = "https://" + addr
	}

	controllerURL, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}

	if controllerURL.Port() == "" {
		controllerURL.Host += fmt.Sprintf(":%d", DefaultControllerPort)
	}
	return controllerURL, nil
}
