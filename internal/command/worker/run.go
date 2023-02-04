package worker

import (
	"github.com/cirruslabs/orchard/internal/worker"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func newRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:  "run",
		RunE: runWorker,
	}
}

func runWorker(cmd *cobra.Command, args []string) (err error) {
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

	worker, err := worker.New(worker.WithDataDirPath(dataDirPath), worker.WithLogger(logger))
	if err != nil {
		return err
	}

	return worker.Run(cmd.Context())
}
