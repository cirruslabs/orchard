package dev

import (
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/config"
	"github.com/cirruslabs/orchard/internal/controller"
	"github.com/cirruslabs/orchard/internal/netconstants"
	"github.com/cirruslabs/orchard/internal/worker"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"os"
	"path"
	"path/filepath"
)

var ErrFailed = errors.New("failed to run development controller and worker")

var devDataDirPath string
var stringToStringResources map[string]string

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "dev",
		Short: "Run a controller and a worker for development purposes",
		RunE:  runDev,
	}

	command.PersistentFlags().StringVarP(&devDataDirPath, "data-dir", "d", ".dev-data",
		"path to persist data between runs")
	command.PersistentFlags().StringToStringVar(&stringToStringResources, "resources", map[string]string{},
		"resources that the development worker will provide")

	return command
}

func runDev(cmd *cobra.Command, args []string) error {
	if !filepath.IsAbs(devDataDirPath) {
		pwd, err := os.Getwd()
		if err != nil {
			return err
		}
		devDataDirPath = path.Join(pwd, devDataDirPath)
	}

	// Convert resources
	resources, err := v1.NewResourcesFromStringToString(stringToStringResources)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFailed, err)
	}

	devController, devWorker, err := CreateDevControllerAndWorker(devDataDirPath,
		fmt.Sprintf(":%d", netconstants.DefaultControllerPort), resources,
		nil, nil)

	if err != nil {
		return err
	}

	errChan := make(chan error, 2)

	go func() {
		if err := devController.Run(cmd.Context()); err != nil {
			errChan <- err
		}
	}()

	go func() {
		if err := devWorker.Run(cmd.Context()); err != nil {
			errChan <- err
		}
	}()

	return <-errChan
}

func CreateDevControllerAndWorker(
	devDataDirPath string,
	controllerListenAddr string,
	resources v1.Resources,
	additionalControllerOpts []controller.Option,
	additionalWorkerOpts []worker.Option,
) (*controller.Controller, *worker.Worker, error) {
	// Initialize the logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if syncErr := logger.Sync(); syncErr != nil && err == nil {
			err = syncErr
		}
	}()

	dataDir, err := controller.NewDataDir(devDataDirPath)
	if err != nil {
		return nil, nil, err
	}

	controllerOpts := []controller.Option{
		controller.WithDataDir(dataDir),
		controller.WithListenAddr(controllerListenAddr),
		controller.WithInsecureAuthDisabled(),
		controller.WithSwaggerDocs(),
		controller.WithLogger(logger),
	}

	controllerOpts = append(controllerOpts, additionalControllerOpts...)

	devController, err := controller.New(controllerOpts...)
	if err != nil {
		return nil, nil, err
	}

	defaultClient, err := client.New(client.WithAddress(devController.Address()))
	if err != nil {
		return nil, nil, err
	}

	workerOpts := []worker.Option{
		worker.WithResources(resources),
		worker.WithLogger(logger),
	}

	workerOpts = append(workerOpts, additionalWorkerOpts...)

	devWorker, err := worker.New(defaultClient, workerOpts...)
	if err != nil {
		return nil, nil, err
	}

	// set local-dev context as active
	configHandle, err := config.NewHandle()
	if err != nil {
		return nil, nil, err
	}
	localContext := config.Context{URL: devController.Address()}
	err = configHandle.CreateContext("local-dev", localContext, true)
	if err != nil {
		return nil, nil, err
	}
	err = configHandle.SetDefaultContext("local-dev")
	if err != nil {
		return nil, nil, err
	}

	return devController, devWorker, nil
}
