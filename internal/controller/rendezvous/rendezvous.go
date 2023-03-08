package rendezvous

import (
	"context"
	"errors"
	"github.com/cirruslabs/orchard/internal/controller/rendezvous/concurrentmap"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("invalid rendez-vous token")
	ErrNoTopic      = errors.New("no topic watchers to rendez-vous with")
)

type Rendezvous[R any, U any] struct {
	requests  *concurrentmap.ConcurrentMap[chan *request[U]]
	responses *concurrentmap.ConcurrentMap[*response[R]]
}

type request[U any] struct {
	Token   string
	Details U
}

type response[R any] struct {
	ctx        context.Context
	resultChan chan R
}

func NewRendezvous[R any, U any]() *Rendezvous[R, U] {
	return &Rendezvous[R, U]{
		requests:  concurrentmap.NewConcurrentMap[chan *request[U]](),
		responses: concurrentmap.NewConcurrentMap[*response[R]](),
	}
}

func (rv *Rendezvous[R, U]) Request(
	ctx context.Context,
	topic string,
	details U,
) (R, error) {
	// Obtain a channel for sending rendez-vous requests to a specific topic watcher
	requestCh, ok := rv.requests.Load(topic)
	if !ok {
		return *new(R), ErrNoTopic
	}

	// Create an entry for receiving rendez-vous responses back from the topic watcher
	token := uuid.New().String()
	response := &response[R]{
		resultChan: make(chan R),
		ctx:        ctx,
	}
	rv.responses.Store(token, response)
	defer rv.responses.Delete(token)

	// Post the rendez-vous request
	request := &request[U]{
		Token:   token,
		Details: details,
	}

	select {
	case requestCh <- request:
		// success, our request was sent, now let's wait for the response
	case <-ctx.Done():
		return *new(R), ctx.Err()
	}

	// Wait for the response
	select {
	case conn := <-response.resultChan:
		return conn, nil
	case <-ctx.Done():
		return *new(R), ctx.Err()
	}
}

func (rv *Rendezvous[R, U]) WatchRequests(topic string) (chan *request[U], func()) {
	requestSlot := make(chan *request[U])
	rv.requests.Store(topic, requestSlot)

	return requestSlot, func() {
		rv.requests.Delete(topic)
	}
}

func (rv *Rendezvous[R, U]) Respond(token string, result R) (context.Context, error) {
	responseSlot, ok := rv.responses.Load(token)
	if !ok {
		return nil, ErrInvalidToken
	}

	responseSlot.resultChan <- result

	return responseSlot.ctx, nil
}
