package proxy

import (
	"context"
	"errors"
	"github.com/cirruslabs/orchard/internal/concurrentmap"
	"net"
)

var ErrInvalidToken = errors.New("invalid proxy token")

type Proxy struct {
	sessions *concurrentmap.ConcurrentMap[*TokenSlot]
}

type TokenSlot struct {
	ctx context.Context
	ch  chan net.Conn
}

func NewProxy() *Proxy {
	return &Proxy{
		sessions: concurrentmap.NewConcurrentMap[*TokenSlot](),
	}
}

func (proxy *Proxy) Request(ctx context.Context, session string) (chan net.Conn, func()) {
	tokenSlot := &TokenSlot{
		ctx: ctx,
		ch:  make(chan net.Conn),
	}

	proxy.sessions.Store(session, tokenSlot)

	return tokenSlot.ch, func() {
		proxy.sessions.Delete(session)
	}
}

func (proxy *Proxy) Respond(session string, conn net.Conn) (context.Context, error) {
	tokenSlot, ok := proxy.sessions.Load(session)
	if !ok {
		return nil, ErrInvalidToken
	}

	tokenSlot.ch <- conn

	return tokenSlot.ctx, nil
}
