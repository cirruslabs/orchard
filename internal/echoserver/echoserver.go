package echoserver

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"syscall"

	"golang.org/x/sync/errgroup"
)

type EchoServer struct {
	listener net.Listener
}

func New() (*EchoServer, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}

	return &EchoServer{
		listener: listener,
	}, nil
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

				for {
					n, err := conn.Read(buf)
					if err != nil {
						if errors.Is(err, io.EOF) {
							return nil
						}

						return err
					}

					_, err = conn.Write(buf[:n])
					if err != nil {
						if errors.Is(err, syscall.EPIPE) {
							return nil
						}

						return err
					}
				}
			})
		}
	})

	return group.Wait()
}
