package controller

import (
	"github.com/cirruslabs/orchard/internal/controller"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func newRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:  "run",
		RunE: runController,
	}
}

func runController(cmd *cobra.Command, args []string) (err error) {
	// Initialize the logger
	logger, err := zap.NewProduction()
	if err != nil {
		return err
	}
	defer func() {
		if syncErr := logger.Sync(); syncErr != nil && err == nil {
			err = syncErr
		}
	}()

	controller, err := controller.New(controller.WithDataDir(dataDir), controller.WithLogger(logger))
	if err != nil {
		return err
	}

	return controller.Run(cmd.Context())
}
