package worker

import "go.uber.org/zap"

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
