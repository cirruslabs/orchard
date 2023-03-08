package rendezvous_test

import (
	"context"
	"github.com/cirruslabs/orchard/internal/controller/rendezvous"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

func TestRendezvous(t *testing.T) {
	rv := rendezvous.NewRendezvous[string, string]()
	expectedResult := uuid.New().String()

	var watchingWg sync.WaitGroup
	var finishedWg sync.WaitGroup

	watchingWg.Add(1)
	finishedWg.Add(1)

	const topic = "worker-name"
	const details = "some data useful for rendez-vous process"

	go func() {
		requestCh, cancel := rv.WatchRequests(topic)
		defer cancel()

		watchingWg.Done()

		request := <-requestCh

		require.Equal(t, details, request.Details)

		ctx, err := rv.Respond(request.Token, expectedResult)
		require.NoError(t, err)

		<-ctx.Done()

		finishedWg.Done()
	}()

	// Wait for the topic subscriber to start listening
	watchingWg.Wait()

	subCtx, cancel := context.WithCancel(context.Background())

	// Request a rendez-vous
	result, err := rv.Request(subCtx, topic, details)
	require.NoError(t, err)
	require.Equal(t, expectedResult, result)

	// Cancel topic subscriber and wait for it to terminate
	cancel()
	finishedWg.Wait()
}
