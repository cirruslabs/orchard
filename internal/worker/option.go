package worker

import (
	"github.com/cirruslabs/orchard/internal/dialer"
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

func WithLabels(labels v1.Labels) Option {
	return func(worker *Worker) {
		worker.labels = labels
	}
}

func WithDefaultCPUAndMemory(defaultCPU uint64, defaultMemory uint64) Option {
	return func(worker *Worker) {
		worker.defaultCPU = defaultCPU
		worker.defaultMemory = defaultMemory
	}
}

func WithDialer(dialer dialer.Dialer) Option {
	return func(worker *Worker) {
		worker.dialer = dialer
	}
}

func WithSynthetic() Option {
	return func(worker *Worker) {
		worker.synthetic = true
	}
}

func WithLogger(logger *zap.Logger) Option {
	return func(worker *Worker) {
		worker.logger = logger.Sugar()
	}
}
