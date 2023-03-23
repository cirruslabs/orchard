package iokitregistry

import (
	"errors"
	"fmt"
	"howett.net/plist"
	"os/exec"
)

var ErrFailed = errors.New("failed to query I/O Kit registry")

type Entry struct {
	IOPlatformUUID string
}

func PlatformUUID() (string, error) {
	ioregPath, err := exec.LookPath("ioreg")
	if err != nil {
		// Fallback since on some systems the PATH
		// variable does not include /usr/sbin
		ioregPath = "/usr/sbin/ioreg"
	}

	ioregCmd := exec.Command(ioregPath, "-a", "-c", "IOPlatformExpertDevice", "-rd1")

	ioregOutput, err := ioregCmd.Output()
	if err != nil {
		return "", fmt.Errorf("%w: failed to run ioreg command: %v", ErrFailed, err)
	}

	var entries []Entry

	_, err = plist.Unmarshal(ioregOutput, &entries)
	if err != nil {
		return "", fmt.Errorf("%w: failed to unmarshal ioreg command's output: %v",
			ErrFailed, err)
	}

	for _, entry := range entries {
		if entry.IOPlatformUUID != "" {
			return entry.IOPlatformUUID, nil
		}
	}

	return "", fmt.Errorf("%w: no platform UUID found in the ioreg command's output",
		ErrFailed)
}
