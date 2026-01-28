package echoserver_test

import (
	"context"
	"crypto/rand"
	"io"
	"net"
	"testing"

	"github.com/cirruslabs/orchard/internal/echoserver"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestServer(t *testing.T) {
	echoServer, err := echoserver.New()
	require.NoError(t, err)

	subCtx, subCtxCancel := context.WithCancel(t.Context())
	group, ctx := errgroup.WithContext(subCtx)
	group.Go(func() error {
		return echoServer.Run(ctx)
	})

	netConn, err := net.Dial("tcp", echoServer.Addr())
	require.NoError(t, err)

	const bufSizeBytes = 64 * 1024
	outgoingBuf := make([]byte, bufSizeBytes)
	incomingBuf := make([]byte, bufSizeBytes)

	// Prepare the outgoing buffer
	n, err := rand.Read(outgoingBuf)
	require.NoError(t, err)
	require.Equal(t, len(outgoingBuf), n)

	// Send the outgoing buffer
	n, err = netConn.Write(outgoingBuf)
	require.NoError(t, err)
	require.Equal(t, len(outgoingBuf), n)

	// Receive the incoming buffer
	n, err = io.ReadFull(netConn, incomingBuf)
	require.NoError(t, err)
	require.Equal(t, len(incomingBuf), n)

	// Compare outgoing and incoming buffers
	require.Equal(t, outgoingBuf, incomingBuf)

	// Ensure clean shutdown
	require.NoError(t, netConn.Close())
	subCtxCancel()
	require.NoError(t, group.Wait())
}
