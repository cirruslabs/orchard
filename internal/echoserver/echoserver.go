package echoserver

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type EchoServer struct {
	listener net.Listener
	logger   *zap.SugaredLogger
}

func New(opts ...Option) (*EchoServer, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}

	echoServer := &EchoServer{
		listener: listener,
	}

	// Apply options
	for _, opt := range opts {
		opt(echoServer)
	}

	// Apply defaults
	if echoServer.logger == nil {
		echoServer.logger = zap.NewNop().Sugar()
	}

	return echoServer, nil
}

func (echoServer *EchoServer) Addr() string {
	return strings.ReplaceAll(echoServer.listener.Addr().String(), "[::]", "127.0.0.1")
}

func (echoServer *EchoServer) Run(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		<-ctx.Done()

		return echoServer.listener.Close()
	})

	group.Go(func() error {
		for {
			conn, err := echoServer.listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return nil
				}

				return err
			}

			group.Go(func() error {
				defer conn.Close()

				buf := make([]byte, 4096)

				_, err := io.CopyBuffer(conn, conn, buf)
				if err != nil {
					if errors.Is(err, io.EOF) {
						return nil
					}

					echoServer.logger.Warnf("connection failed: %v", err)
				}

				return nil
			})
		}
	})

	return group.Wait()
}
