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
	rejectFirstSessions    atomic.Int32
	successfulConnections  atomic.Int32
	keepaliveRequests      atomic.Int32

	mu    sync.Mutex
	conns map[net.Conn]struct{}

	wg sync.WaitGroup
}

func startExecSSHServer(t *testing.T, rejectFirstConnections int32) *execSSHServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	signer, err := ssh.NewSignerFromKey(privateKey)
	require.NoError(t, err)

	server := &execSSHServer{
		listener: listener,
		conns:    map[net.Conn]struct{}{},
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
		require.NoError(t, server.listener.Close())
		server.wg.Wait()
	})

	return server
}

func (server *execSSHServer) Addr() string {
	return server.listener.Addr().String()
}

func (server *execSSHServer) SuccessfulConnections() int32 {
	return server.successfulConnections.Load()
}

func (server *execSSHServer) KeepaliveRequests() int32 {
	return server.keepaliveRequests.Load()
}

func (server *execSSHServer) RejectNextSessions(count int32) {
	server.rejectFirstSessions.Store(count)
}

func (server *execSSHServer) CloseClientConnections() {
	server.mu.Lock()
	conns := make([]net.Conn, 0, len(server.conns))
	for conn := range server.conns {
		conns = append(conns, conn)
	}
	server.mu.Unlock()

	for _, conn := range conns {
		_ = conn.Close()
	}
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
	server.mu.Lock()
	server.conns[conn] = struct{}{}
	server.mu.Unlock()

	defer func() {
		server.mu.Lock()
		delete(server.conns, conn)
		server.mu.Unlock()
	}()
	defer conn.Close()

	serverConn, newChannels, requests, err := ssh.NewServerConn(conn, server.config)
	if err != nil {
		return
	}
	defer serverConn.Close()

	server.successfulConnections.Add(1)

	go server.serveGlobalRequests(requests)

	for newChannel := range newChannels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")

			continue
		}
		if server.rejectFirstSessions.Add(-1) >= 0 {
			_ = newChannel.Reject(ssh.Prohibited, "session rejected for test")

			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}

		server.wg.Add(1)
		go func() {
			defer server.wg.Done()

			serveExecSSHSession(channel, requests)
		}()
	}
}

func (server *execSSHServer) serveGlobalRequests(requests <-chan *ssh.Request) {
	for request := range requests {
		if request.Type == "keepalive@openssh.com" {
			server.keepaliveRequests.Add(1)
		}
		if request.WantReply {
			_ = request.Reply(false, nil)
		}
	}
}

func serveExecSSHSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()

	for request := range requests {
		switch request.Type {
		case "exec":
			_ = request.Reply(true, nil)
			_, _ = io.WriteString(channel, "ok")
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct {
				Status uint32
			}{Status: 0}))

			return
		default:
			_ = request.Reply(false, nil)
		}
	}
}
