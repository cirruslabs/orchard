package platform

import "github.com/cirruslabs/orchard/internal/worker/platform/iokitregistry"

func MachineID() (string, error) {
	return iokitregistry.PlatformUUID()
}
