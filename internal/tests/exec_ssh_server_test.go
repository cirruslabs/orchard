package tests_test

import (
	"crypto/ed25519"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

type execSSHServer struct {
	listener net.Listener
	config   *ssh.ServerConfig

	rejectFirstConnections atomic.Int32
	successfulConnections  atomic.Int32
	acceptedSessions       atomic.Int32
	releaseSessions        <-chan struct{}
	done                   chan struct{}

	wg sync.WaitGroup
}

func startExecSSHServer(t *testing.T, rejectFirstConnections int32) *execSSHServer {
	return startExecSSHServerWithSessionGate(t, rejectFirstConnections, nil)
}

func startExecSSHServerWithSessionGate(
	t *testing.T,
	rejectFirstConnections int32,
	releaseSessions <-chan struct{},
) *execSSHServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	signer, err := ssh.NewSignerFromKey(privateKey)
	require.NoError(t, err)

	server := &execSSHServer{
		listener:        listener,
		releaseSessions: releaseSessions,
		done:            make(chan struct{}),
		config: &ssh.ServerConfig{
			PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
				if conn.User() != "admin" || string(password) != "admin" {
					return nil, ssh.ErrNoAuth
				}

				return &ssh.Permissions{}, nil
			},
		},
	}
	server.rejectFirstConnections.Store(rejectFirstConnections)
	server.config.AddHostKey(signer)

	server.wg.Add(1)
	go server.run()

	t.Cleanup(func() {
		close(server.done)
		require.NoError(t, server.listener.Close())
		server.wg.Wait()
	})

	return server
}

func (server *execSSHServer) Addr() string {
	return server.listener.Addr().String()
}

func (server *execSSHServer) run() {
	defer server.wg.Done()

	for {
		conn, err := server.listener.Accept()
		if err != nil {
			return
		}

		if server.rejectFirstConnections.Add(-1) >= 0 {
			_ = conn.Close()

			continue
		}

		server.wg.Add(1)
		go func() {
			defer server.wg.Done()

			server.serve(conn)
		}()
	}
}

func (server *execSSHServer) serve(conn net.Conn) {
	defer conn.Close()

	serverConn, newChannels, requests, err := ssh.NewServerConn(conn, server.config)
	if err != nil {
		return
	}
	server.successfulConnections.Add(1)
	defer serverConn.Close()

	go ssh.DiscardRequests(requests)

	for newChannel := range newChannels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")

			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		server.acceptedSessions.Add(1)

		server.wg.Add(1)
		go func() {
			defer server.wg.Done()

			server.serveExecSSHSession(channel, requests)
		}()
	}
}

func (server *execSSHServer) serveExecSSHSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()

	for request := range requests {
		switch request.Type {
		case "exec":
			_ = request.Reply(true, nil)
			_, _ = io.WriteString(channel, "ok")
			if server.releaseSessions != nil {
				select {
				case <-server.releaseSessions:
				case <-server.done:
					return
				}
			}
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct {
				Status uint32
			}{Status: 0}))

			return
		default:
			_ = request.Reply(false, nil)
		}
	}
}
