package echoserver

import "go.uber.org/zap"

type Option func(echoServer *EchoServer)

func WithLogger(logger *zap.SugaredLogger) Option {
	return func(echoServer *EchoServer) {
		echoServer.logger = logger
	}
}
