package dev

import (
	"github.com/cirruslabs/orchard/internal/controller"
	"github.com/cirruslabs/orchard/internal/worker"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"os"
	"path"
	"path/filepath"
)

var devDataDirPath string

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "dev",
		Short: "Run a controller and a worker for development purposes",
		RunE:  runDev,
	}

	command.PersistentFlags().StringVarP(&devDataDirPath, "data-dir", "d", ".dev-data",
		"path to persist data between runs")

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

	if !filepath.IsAbs(devDataDirPath) {
		pwd, err := os.Getwd()
		if err != nil {
			return err
		}
		devDataDirPath = path.Join(pwd, devDataDirPath)
	}

	dataDir, err := controller.NewDataDir(devDataDirPath)
	if err != nil {
		return err
	}

	controller, err := controller.New(controller.WithDataDir(dataDir),
		controller.WithInsecureAuthDisabled(), controller.WithLogger(logger))
	if err != nil {
		return err
	}

	worker, err := worker.New(worker.WithDataDirPath(devDataDirPath), worker.WithLogger(logger))
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
