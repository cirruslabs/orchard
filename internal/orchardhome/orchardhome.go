package orchardhome

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrFailed = errors.New("failed to retrieve Orchard's home directory path")

func Path() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%w: failed to retrieve current user's home directory %v",
			ErrFailed, err)
	}

	orchardDir := filepath.Join(homeDir, ".orchard")

	if err := os.Mkdir(orchardDir, 0700); err != nil && !os.IsExist(err) {
		return "", fmt.Errorf("%w: cannot create directory %s: %v",
			ErrFailed, orchardDir, err)
	}

	return orchardDir, nil
}
