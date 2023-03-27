package worker

import (
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/bootstraptoken"
	"github.com/cirruslabs/orchard/internal/netconstants"
	"github.com/cirruslabs/orchard/internal/worker"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var ErrRunFailed = errors.New("failed to run worker")

var ErrBootstrapTokenNotProvided = errors.New("no bootstrap token provided")

var bootstrapTokenRaw string
var logFilePath string
var stringToStringResources map[string]string

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run CONTROLLER_URL",
		Short: "Run worker",
		RunE:  runWorker,
		Args:  cobra.ExactArgs(1),
	}

	cmd.PersistentFlags().StringVar(&bootstrapTokenRaw, "bootstrap-token", "",
		"a bootstrap token retrieved via `orchard get bootstrap-token <service-account-name-for-workers>`")
	cmd.PersistentFlags().StringVar(&logFilePath, "log-file", "",
		"optional path to a file where logs (up to 100 Mb) will be written.")
	cmd.PersistentFlags().StringToStringVar(&stringToStringResources, "resources", map[string]string{},
		"resources that this worker provides")

	return cmd
}

func runWorker(cmd *cobra.Command, args []string) (err error) {
	controllerURL, err := netconstants.NormalizeAddress(args[0])
	if err != nil {
		return err
	}
	if bootstrapTokenRaw == "" {
		return ErrBootstrapTokenNotProvided
	}
	bootstrapToken, err := bootstraptoken.NewFromString(bootstrapTokenRaw)
	if err != nil {
		return err
	}

	// Convert resources
	resources, err := v1.NewResourcesFromStringToString(stringToStringResources)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRunFailed, err)
	}

	controllerClient, err := client.New(
		client.WithAddress(controllerURL.String()),
		client.WithTrustedCertificate(bootstrapToken.Certificate()),
		client.WithCredentials(bootstrapToken.ServiceAccountName(), bootstrapToken.ServiceAccountToken()),
	)
	if err != nil {
		return err
	}

	// Initialize the logger
	logger, err := createLogger()
	if err != nil {
		return err
	}

	defer func() {
		if syncErr := logger.Sync(); syncErr != nil && err == nil {
			err = syncErr
		}
	}()

	workerInstance, err := worker.New(
		controllerClient,
		worker.WithResources(resources),
		worker.WithLogger(logger),
	)
	if err != nil {
		return err
	}

	return workerInstance.Run(cmd.Context())
}

func createLogger() (*zap.Logger, error) {
	if logFilePath == "" {
		return zap.NewProduction()
	}

	logFileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename: logFilePath,
		MaxSize:  100, // megabytes
	})
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		logFileWriter,
		zap.InfoLevel,
	)
	return zap.New(core), nil
}
