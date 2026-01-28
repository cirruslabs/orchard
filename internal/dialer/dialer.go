package dialer

import (
	"context"
	"net"
)

type Dialer interface {
	Dial(network, addr string) (net.Conn, error)
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func (f DialFunc) Dial(network, addr string) (net.Conn, error) {
	return f(context.Background(), network, addr)
}

func (f DialFunc) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return f(ctx, network, addr)
}
