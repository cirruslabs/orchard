package worker

import (
	"go.uber.org/zap"
)

type Option func(*Worker)

func WithLogger(logger *zap.Logger) Option {
	return func(worker *Worker) {
		worker.logger = logger.Sugar()
	}
}
