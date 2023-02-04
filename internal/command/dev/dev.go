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

	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		return err
	}

	dataDir, err := controller.NewDataDir(tempDir)
	if err != nil {
		return err
	}

	controller, err := controller.New(controller.WithDataDir(dataDir), controller.WithLogger(logger))
	if err != nil {
		return err
	}

	worker, err := worker.New(worker.WithDataDirPath(tempDir), worker.WithLogger(logger))
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
