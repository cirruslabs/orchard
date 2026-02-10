package sshexec_test

import (
	"net"
	"testing"
	"time"

	"github.com/cirruslabs/orchard/internal/controller/sshexec"
	"github.com/stretchr/testify/require"
)

func TestContextCancellationViaNetConnClose(t *testing.T) {
	clientConn, serverConn := net.Pipe()

	go func() {
		select {
		case <-t.Context().Done():
			return
		case <-time.After(5 * time.Second):
			require.NoError(t, serverConn.Close())
		}
	}()

	_, err := sshexec.New(clientConn, "doesn't", "matter", false)
	require.Error(t, err)
}
