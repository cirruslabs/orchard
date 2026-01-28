package platform

import (
	"os"
)

func MachineID() (string, error) {
	machineIDBytes, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return "", err
	}

	return string(machineIDBytes), nil
}
