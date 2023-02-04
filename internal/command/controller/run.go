package controller

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/controller"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var ErrRunFailed = errors.New("failed to run controller")

func newRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the controller",
		RunE:  runController,
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

	// Instantiate a data directory and ensure it's initialized
	dataDir, err := controller.NewDataDir(dataDirPath)
	if err != nil {
		return err
	}

	initialized, err := dataDir.Initialized()
	if err != nil {
		return err
	}

	if !initialized {
		return fmt.Errorf("%w: data directory is not initialized, please run \"orchard controller init\" first",
			ErrRunFailed)
	}

	controllerCert, err := dataDir.ControllerCertificate()
	if err != nil {
		return err
	}

	controller, err := controller.New(
		controller.WithDataDir(dataDir),
		controller.WithLogger(logger),
		controller.WithTLSConfig(&tls.Config{
			MinVersion: tls.VersionTLS13,
			Certificates: []tls.Certificate{
				controllerCert,
			},
		}),
	)
	if err != nil {
		return err
	}

	return controller.Run(cmd.Context())
}
