package controller

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/controller"
	"github.com/cirruslabs/orchard/internal/netconstants"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"os"
	"strconv"
)

var ErrRunFailed = errors.New("failed to run controller")

var address string

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the controller",
		RunE:  runController,
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = strconv.FormatInt(netconstants.DefaultControllerPort, 10)
	}

	cmd.PersistentFlags().StringVarP(&address, "listen", "l", fmt.Sprintf(":%s", port), "address to listen on")

	return cmd
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
		controller.WithListenAddr(address),
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
