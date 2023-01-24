package controller

import "path/filepath"

func (controller *Controller) dbPath() string {
	return filepath.Join(controller.dataDir, "db")
}

func (controller *Controller) caCertPath() string {
	return filepath.Join(controller.dataDir, "ca.crt")
}

func (controller *Controller) caKeyPath() string {
	return filepath.Join(controller.dataDir, "ca.key")
}
