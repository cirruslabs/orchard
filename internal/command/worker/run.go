package worker

import (
	"errors"
	"github.com/cirruslabs/orchard/internal/bootstraptoken"
	"github.com/cirruslabs/orchard/internal/netconstants"
	"github.com/cirruslabs/orchard/internal/worker"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var ErrBootstrapTokenNotProvided = errors.New("no bootstrap token provided")

var bootstrapTokenRaw string
var logFilePath string

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run CONTROLLER_URL",
		Short: "Run worker",
		RunE:  runWorker,
		Args:  cobra.ExactArgs(1),
	}
	cmd.PersistentFlags().StringVar(&bootstrapTokenRaw, "bootstrap-token", "",
		"a bootstrap token retrieved via `orchard get bootstrap-token <service-account-name-for-workers>`")
	cmd.PersistentFlags().StringVar(&logFilePath, "log-file", "/tmp/orchard-worker.log",
		"path to a file where logs (up to 100 Mb) will be written.")
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

	controllerClient, err := client.New(
		client.WithAddress(controllerURL.String()),
		client.WithTrustedCertificate(bootstrapToken.Certificate()),
		client.WithCredentials(bootstrapToken.ServiceAccountName(), bootstrapToken.ServiceAccountToken()),
	)
	if err != nil {
		return err
	}

	// Initialize the logger
	logFileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename: logFilePath,
		MaxSize:  100, // megabytes
	})
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		logFileWriter,
		zap.InfoLevel,
	)
	logger := zap.New(core)

	if err != nil {
		return err
	}
	defer func() {
		if syncErr := logger.Sync(); syncErr != nil && err == nil {
			err = syncErr
		}
	}()

	workerInstance, err := worker.New(controllerClient, worker.WithLogger(logger))
	if err != nil {
		return err
	}

	return workerInstance.Run(cmd.Context())
}
