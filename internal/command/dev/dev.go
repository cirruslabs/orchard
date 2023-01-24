package dev

import (
	"github.com/cirruslabs/orchard/internal/controller"
	"github.com/cirruslabs/orchard/internal/worker"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"os"
)

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "dev",
		Short: "Run a controller and a worker for development purposes",
		RunE:  runDev,
	}

	return command
}

func runDev(cmd *cobra.Command, args []string) error {
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		return err
	}

	// Initialize the logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		return err
	}
	defer func() {
		if syncErr := logger.Sync(); syncErr != nil && err == nil {
			err = syncErr
		}
	}()

	controller, err := controller.New(controller.WithDataDir(tempDir), controller.WithLogger(logger))
	if err != nil {
		return err
	}

	worker, err := worker.New(worker.WithDataDir(tempDir), worker.WithLogger(logger))
	if err != nil {
		return err
	}

	errChan := make(chan error, 2)

	go func() {
		if err := controller.Run(cmd.Context()); err != nil {
			errChan <- err
		}
	}()

	go func() {
		if err := worker.Run(cmd.Context()); err != nil {
			errChan <- err
		}
	}()

	return <-errChan
}
