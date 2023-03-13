package rendezvous

import (
	"context"
	"errors"
	"github.com/cirruslabs/orchard/internal/controller/rendezvous/concurrentmap"
	"net"
)

var ErrInvalidToken = errors.New("invalid proxy token")

type Proxy struct {
	tokens *concurrentmap.ConcurrentMap[*TokenSlot]
}

type TokenSlot struct {
	ctx context.Context
	ch  chan net.Conn
}

func NewProxy() *Proxy {
	return &Proxy{
		tokens: concurrentmap.NewConcurrentMap[*TokenSlot](),
	}
}

func (proxy *Proxy) Request(ctx context.Context, token string) (chan net.Conn, func()) {
	tokenSlot := &TokenSlot{
		ctx: ctx,
		ch:  make(chan net.Conn),
	}

	proxy.tokens.Store(token, tokenSlot)

	return tokenSlot.ch, func() {
		proxy.tokens.Delete(token)
	}
}

func (proxy *Proxy) Respond(token string, conn net.Conn) (context.Context, error) {
	tokenSlot, ok := proxy.tokens.Load(token)
	if !ok {
		return nil, ErrInvalidToken
	}

	tokenSlot.ch <- conn

	return tokenSlot.ctx, nil
}
