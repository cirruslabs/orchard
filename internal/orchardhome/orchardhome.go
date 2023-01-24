package orchardhome

import (
	"os"
	"path/filepath"
)

func Path() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	orchardDir := filepath.Join(homeDir, ".orchard")

	if err := os.Mkdir(orchardDir, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}

	return orchardDir, nil
}
