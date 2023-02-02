package controller

import (
	"crypto/tls"
	"go.uber.org/zap"
)

type Option func(*Controller)

func WithDataDir(dataDir *DataDir) Option {
	return func(controller *Controller) {
		controller.dataDir = dataDir
	}
}

func WithListenAddr(listenAddr string) Option {
	return func(controller *Controller) {
		controller.listenAddr = listenAddr
	}
}

func WithTLSConfig(tlsConfig *tls.Config) Option {
	return func(controller *Controller) {
		controller.tlsConfig = tlsConfig
	}
}

func WithLogger(logger *zap.Logger) Option {
	return func(controller *Controller) {
		controller.logger = logger.Sugar()
	}
}
