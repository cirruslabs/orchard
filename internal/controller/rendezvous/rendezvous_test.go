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

// TestProxyNonBlockingRespond ensures that the Respond() won't block
// the caller in the absense of the receiving party.
func TestProxyNonBlockingRespond(t *testing.T) {
	ctx := context.Background()

	expectedConn, _ := net.Pipe()

	proxy := rendezvous.New[net.Conn]()

	token := uuid.New().String()

	connCh, cancel := proxy.Request(ctx, token)
	defer cancel()

	// Call Respond() in the same goroutine as Request()
	_, err := proxy.Respond(token, expectedConn)
	require.NoError(t, err)

	actualConn := <-connCh
	require.Equal(t, expectedConn, actualConn)
}

// TestProxyDoubleRespond ensures that the Respond() can be
// safely called multiple times.
func TestProxyDoubleRespond(t *testing.T) {
	ctx := context.Background()

	expectedConn, _ := net.Pipe()

	proxy := rendezvous.New[net.Conn]()

	token := uuid.New().String()

	_, cancel := proxy.Request(ctx, token)
	defer cancel()

	// Call Respond() twice
	_, err := proxy.Respond(token, expectedConn)
	require.NoError(t, err)

	_, err = proxy.Respond(token, expectedConn)
	require.NoError(t, err)
}
