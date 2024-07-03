package rendezvous_test

import (
	"context"
	"github.com/cirruslabs/orchard/internal/controller/rendezvous"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"net"
	"sync"
	"testing"
)

func TestProxy(t *testing.T) {
	ctx := context.Background()

	expectedConn, _ := net.Pipe()

	proxy := rendezvous.New[net.Conn]()

	token := uuid.New().String()

	connCh, cancel := proxy.Request(ctx, token)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		_, err := proxy.Respond(token, expectedConn)
		require.NoError(t, err)

		wg.Done()
	}()

	actualConn := <-connCh
	require.Equal(t, expectedConn, actualConn)

	wg.Wait()
}
