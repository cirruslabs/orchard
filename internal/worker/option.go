package worker

import (
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"go.uber.org/zap"
)

type Option func(*Worker)

func WithName(name string) Option {
	return func(worker *Worker) {
		worker.name = name
	}
}

func WithResources(resources v1.Resources) Option {
	return func(worker *Worker) {
		worker.resources = resources
	}
}

func WithLogger(logger *zap.Logger) Option {
	return func(worker *Worker) {
		worker.logger = logger.Sugar()
	}
}
