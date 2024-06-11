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
var addressSSH string
var debug bool

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

	cmd.PersistentFlags().StringVarP(&address, "listen", "l", fmt.Sprintf(":%s", port),
		"address to listen on")
	cmd.PersistentFlags().StringVar(&addressSSH, "listen-ssh", "",
		"address for the built-in SSH server to listen on (e.g. \":6122\")")
	cmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")

	// flags for auto-init if necessary
	// this simplifies the user experience to run the controller in serverless environments
	cmd.PersistentFlags().StringVar(&controllerCertPath, "controller-cert", "",
		"use the controller certificate from the specified path instead of the auto-generated one"+
			" (requires --controller-key)")
	cmd.PersistentFlags().StringVar(&controllerKeyPath, "controller-key", "",
		"use the controller certificate key from the specified path instead of the auto-generated one"+
			" (requires --controller-cert)")
	cmd.PersistentFlags().StringVar(&sshHostKeyPath, "ssh-host-key", "",
		"use the SSH private host key from the specified path instead of the auto-generated one")

	return cmd
}

func runController(cmd *cobra.Command, args []string) (err error) {
	// Initialize the logger
	zapConfig := zap.NewProductionConfig()
	if debug {
		zapConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}
	logger, err := zapConfig.Build()
	if err != nil {
		return err
	}
	defer func() {
		if syncErr := logger.Sync(); syncErr != nil && err == nil {
			err = syncErr
		}
	}()

	// Redirect standard's library package-global logger
	// to our zap logger at debug level
	if _, err := zap.RedirectStdLogAt(logger, zap.DebugLevel); err != nil {
		return err
	}

	// Instantiate a data directory and ensure it's initialized
	dataDir, err := controller.NewDataDir(dataDirPath)
	if err != nil {
		return err
	}

	controllerCert, err := FindControllerCertificate(dataDir)
	if err != nil {
		return err
	}

	controllerOpts := []controller.Option{
		controller.WithListenAddr(address),
		controller.WithDataDir(dataDir),
		controller.WithLogger(logger),
		controller.WithTLSConfig(&tls.Config{
			MinVersion: tls.VersionTLS12,
			Certificates: []tls.Certificate{
				controllerCert,
			},
		}),
	}

	if addressSSH != "" {
		signer, err := FindSSHHostKey(dataDir)
		if err != nil {
			return err
		}

		controllerOpts = append(controllerOpts, controller.WithSSHServer(addressSSH, signer))
	}

	controllerInstance, err := controller.New(controllerOpts...)
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
