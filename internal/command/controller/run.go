package controller

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/controller"
	"github.com/cirruslabs/orchard/internal/netconstants"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"os"
	"strconv"
)

var ErrRunFailed = errors.New("failed to run controller")
var BootstrapAdminAccountName = "bootstrap-admin"

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

	// flags for auto-init if necessary
	// this simplifies the user experience to run the controller in serverless environments
	cmd.PersistentFlags().StringVar(&controllerCertPath, "controller-cert", "",
		"use the controller certificate from the specified path instead of the auto-generated one"+
			" (requires --controller-key)")
	cmd.PersistentFlags().StringVar(&controllerKeyPath, "controller-key", "",
		"use the controller certificate key from the specified path instead of the auto-generated one"+
			" (requires --controller-cert)")

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

	var controllerCert tls.Certificate
	if dataDir.ControllerCertificateExists() {
		controllerCert, err = dataDir.ControllerCertificate()
		if err != nil {
			return err
		}
	} else {
		controllerCert, err = FindControllerCertificate(dataDir)
		if err != nil {
			return err
		}
	}

	controllerInstance, err := controller.New(
		controller.WithListenAddr(address),
		controller.WithDataDir(dataDir),
		controller.WithLogger(logger),
		controller.WithTLSConfig(&tls.Config{
			MinVersion: tls.VersionTLS12,
			Certificates: []tls.Certificate{
				controllerCert,
			},
		}),
	)
	if err != nil {
		return err
	}

	if adminToken, ok := os.LookupEnv("ORCHARD_BOOTSTRAP_ADMIN_TOKEN"); ok {
		err = controllerInstance.EnsureServiceAccount(&v1.ServiceAccount{
			Meta: v1.Meta{
				Name: BootstrapAdminAccountName,
			},
			Token: adminToken,
			Roles: v1.AllServiceAccountRoles(),
		})
	} else {
		err = controllerInstance.DeleteServiceAccount(BootstrapAdminAccountName)
	}
	if err != nil {
		return err
	}

	return controllerInstance.Run(cmd.Context())
}
