package rendezvous

import (
	"context"
	"errors"
	"github.com/cirruslabs/orchard/internal/concurrentmap"
)

var ErrInvalidToken = errors.New("invalid rendezvous token")

type Rendezvous[T any] struct {
	sessions *concurrentmap.ConcurrentMap[*TokenSlot[T]]
}

type TokenSlot[T any] struct {
	ctx context.Context
	ch  chan T
}

func New[T any]() *Rendezvous[T] {
	return &Rendezvous[T]{
		sessions: concurrentmap.NewConcurrentMap[*TokenSlot[T]](),
	}
}

func (rendezvous *Rendezvous[T]) Request(ctx context.Context, session string) (chan T, func()) {
	tokenSlot := &TokenSlot[T]{
		ctx: ctx,
		ch:  make(chan T, 1),
	}

	rendezvous.sessions.Store(session, tokenSlot)

	return tokenSlot.ch, func() {
		rendezvous.sessions.Delete(session)
	}
}

func (rendezvous *Rendezvous[T]) Respond(session string, conn T) (context.Context, error) {
	tokenSlot, ok := rendezvous.sessions.Load(session)
	if !ok {
		return nil, ErrInvalidToken
	}

	select {
	case tokenSlot.ch <- conn:
		// first response
	default:
		// the receiving party still hadn't picked up the previous response, so discard the new one
	}

	return tokenSlot.ctx, nil
}
