package worker

import (
	"github.com/cirruslabs/orchard/pkg/client"
	"go.uber.org/zap"
)

type Option func(*Worker)

func WithDataDirPath(dataDir string) Option {
	return func(worker *Worker) {
		worker.dataDirPath = dataDir
	}
}

func WithLogger(logger *zap.Logger) Option {
	return func(worker *Worker) {
		worker.logger = logger.Sugar()
	}
}

func WithClient(client *client.Client) Option {
	return func(worker *Worker) {
		worker.client = client
	}
}
