package controller

import "path/filepath"

func (controller *Controller) dbPath() string {
	return filepath.Join(controller.dataDir, "db")
}
