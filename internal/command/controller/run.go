package controller

import (
	"crypto/tls"
	"encoding/pem"
	"errors"
	"fmt"
	configpkg "github.com/cirruslabs/orchard/internal/config"
	"github.com/cirruslabs/orchard/internal/controller"
	"github.com/cirruslabs/orchard/internal/netconstants"
	"github.com/cirruslabs/orchard/internal/orchardhome"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

var ErrRunFailed = errors.New("failed to run controller")

var address string
var addressSSH string
var debug bool
var noTLS bool
var sshNoClientAuth bool
var experimentalRPCV2 bool
var noExperimentalRPCV2 bool
var experimentalPingInterval time.Duration

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
	cmd.PersistentFlags().BoolVar(&noTLS, "insecure-no-tls", false,
		"disable TLS, making all connections to the controller unencrypted")
	cmd.PersistentFlags().BoolVar(&sshNoClientAuth, "insecure-ssh-no-client-auth", false,
		"allow SSH clients to connect to the controller's SSH server without authentication, "+
			"thus only authenticating on the target worker/VM's SSH server")
	cmd.PersistentFlags().BoolVar(&experimentalRPCV2, "experimental-rpc-v2", false,
		"enable experimental RPC v2 (https://github.com/cirruslabs/orchard/issues/235)")
	_ = cmd.PersistentFlags().MarkHidden("experimental-rpc-v2")
	cmd.PersistentFlags().BoolVar(&noExperimentalRPCV2, "no-experimental-rpc-v2", false,
		"disable experimental RPC v2 (https://github.com/cirruslabs/orchard/issues/235)")
	cmd.PersistentFlags().DurationVar(&experimentalPingInterval, "experimental-ping-interval", 0,
		"interval between WebSocket PING's sent by the controller to workers and clients, "+
			"useful when facing intermediate load balancers/proxies that have timeouts "+
			"smaller than the controller's default 30 second interval")

	return cmd
}

func runController(cmd *cobra.Command, args []string) (err error) {
	// Avoid fancy output that doesn't go through the logger
	gin.SetMode(gin.ReleaseMode)

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
	if dataDirPath == "" {
		orchardHome, err := orchardhome.Path()
		if err != nil {
			return err
		}

		dataDirPath = filepath.Join(orchardHome, "controller")
	}

	dataDir, err := controller.NewDataDir(dataDirPath)
	if err != nil {
		return err
	}

	controllerOpts := []controller.Option{
		controller.WithListenAddr(address),
		controller.WithDataDir(dataDir),
		controller.WithLogger(logger),
	}

	var controllerCert tls.Certificate

	if !noTLS {
		controllerCert, err = FindControllerCertificate(dataDir)
		if err != nil {
			return err
		}

		controllerOpts = append(controllerOpts, controller.WithTLSConfig(&tls.Config{
			MinVersion: tls.VersionTLS12,
			Certificates: []tls.Certificate{
				controllerCert,
			},
			// Since gRPC clients started enforcing ALPN at some point, we need to advertise it
			//
			// See https://github.com/grpc/grpc-go/issues/7922 for more details.
			NextProtos: []string{"http/1.1", "h2"},
		}))
	}

	if addressSSH != "" {
		signer, err := FindSSHHostKey(dataDir)
		if err != nil {
			return err
		}

		controllerOpts = append(controllerOpts, controller.WithSSHServer(addressSSH, signer, sshNoClientAuth))
	}

	if experimentalRPCV2 && noExperimentalRPCV2 {
		return fmt.Errorf("--experimental-rpc-v2 and --no-experimental-rpc-v2 flags are mutually exclusive")
	}

	if experimentalRPCV2 {
		logger.Warn("--experimental-rpc-v2 flag is deprecated: experimental RPC v2 is now enabled by default")
	}

	if !noExperimentalRPCV2 {
		controllerOpts = append(controllerOpts, controller.WithExperimentalRPCV2())
	}

	if experimentalPingInterval != 0 {
		controllerOpts = append(controllerOpts, controller.WithPingInterval(experimentalPingInterval))
	}

	controllerInstance, err := controller.New(controllerOpts...)
	if err != nil {
		return err
	}

	// Return bootstrap service account credentials, optionally creating it
	serviceAccountName, serviceAccountToken, err := Bootstrap(controllerInstance, controllerCert)
	if err != nil {
		return err
	}

	// If bootstrap service account still exists, update a context for it,
	// optionally making this context the default one
	if serviceAccountName != "" && serviceAccountToken != "" {
		if err := createBootstrapContext(controllerInstance.Address(), controllerCert,
			serviceAccountName, serviceAccountToken); err != nil {
			return err
		}
	}

	return controllerInstance.Run(cmd.Context())
}

func createBootstrapContext(
	controllerAddress string,
	controllerCert tls.Certificate,
	serviceAccountName string,
	serviceAccountToken string,
) error {
	configHandle, err := configpkg.NewHandle()
	if err != nil {
		return err
	}

	context := configpkg.Context{
		URL:                 controllerAddress,
		ServiceAccountName:  serviceAccountName,
		ServiceAccountToken: serviceAccountToken,
	}

	if !noTLS {
		certificatePEMBytes := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: controllerCert.Certificate[0],
		})

		context.Certificate = certificatePEMBytes
	}

	if err := configHandle.CreateContext(BootstrapContextName, context, true); err != nil {
		return err
	}

	config, err := configHandle.Config()
	if err != nil {
		return err
	}

	if config.DefaultContext == "" {
		if err := configHandle.SetDefaultContext(BootstrapContextName); err != nil {
			return err
		}
	}

	return nil
}
