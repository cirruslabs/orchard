package controller

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cirruslabs/orchard/internal/controller/sshexec"
	"go.uber.org/zap"
)

var errExecSSHClientPoolClosed = errors.New("SSH exec client pool is closed")

type execSSHClientPool struct {
	// clients maps VM UID strings to *pooledExecSSHClient values so each VM has
	// at most one shared SSH connection.
	clients sync.Map

	closed atomic.Bool

	keepaliveInterval time.Duration
	logger            *zap.SugaredLogger
}

func newExecSSHClientPool(keepaliveInterval time.Duration, logger *zap.SugaredLogger) *execSSHClientPool {
	return &execSSHClientPool{
		keepaliveInterval: keepaliveInterval,
		logger:            logger,
	}
}

func (pool *execSSHClientPool) newExec(
	vmUID string,
	options sshexec.Options,
	create func() (sshExecClient, error),
) (*sshexec.Exec, error) {
	if pool.closed.Load() {
		return nil, errExecSSHClientPoolClosed
	}

	return pool.client(vmUID).newExec(func() (sshExecClient, error) {
		initializedClient, err := create()
		if err != nil {
			return nil, err
		}
		if pool.closed.Load() {
			_ = initializedClient.Close()

			return nil, errExecSSHClientPoolClosed
		}

		return initializedClient, nil
	}, options)
}

func (pool *execSSHClientPool) closeAll() {
	pool.closed.Store(true)

	pool.clients.Range(func(key any, value any) bool {
		pool.clients.Delete(key)
		value.(*pooledExecSSHClient).close()

		return true
	})
}

func (pool *execSSHClientPool) client(vmUID string) *pooledExecSSHClient {
	client, _ := pool.clients.LoadOrStore(vmUID, &pooledExecSSHClient{
		vmUID:             vmUID,
		keepaliveInterval: pool.keepaliveInterval,
		logger:            pool.logger,
	})

	return client.(*pooledExecSSHClient)
}

type pooledExecSSHClient struct {
	vmUID string

	mu      sync.Mutex
	current sshExecClient

	keepaliveInterval time.Duration
	logger            *zap.SugaredLogger
}

func (client *pooledExecSSHClient) newExec(
	create func() (sshExecClient, error),
	options sshexec.Options,
) (*sshexec.Exec, error) {
	current, err := client.getOrInit(create)
	if err != nil {
		return nil, err
	}

	exec, err := current.NewExec(options)
	if err != nil && current.ShouldRecreateAfter(err) {
		client.invalidate(current)
	}

	return exec, err
}

func (client *pooledExecSSHClient) getOrInit(
	create func() (sshExecClient, error),
) (sshExecClient, error) {
	client.mu.Lock()
	defer client.mu.Unlock()

	if client.current != nil {
		return client.current, nil
	}

	initializedClient, err := create()
	if err != nil {
		return nil, err
	}

	client.current = initializedClient
	client.monitor(initializedClient)

	return initializedClient, nil
}

func (client *pooledExecSSHClient) invalidate(expected sshExecClient) bool {
	if !client.clear(expected) {
		return false
	}

	_ = expected.Close()

	return true
}

func (client *pooledExecSSHClient) clear(expected sshExecClient) bool {
	client.mu.Lock()
	defer client.mu.Unlock()

	if client.current != expected {
		return false
	}

	client.current = nil

	return true
}

func (client *pooledExecSSHClient) close() {
	client.mu.Lock()
	current := client.current
	client.current = nil
	client.mu.Unlock()

	if current == nil {
		return
	}

	_ = current.Close()
}

func (client *pooledExecSSHClient) monitor(current sshExecClient) {
	go func() {
		var keepalive <-chan time.Time
		if client.keepaliveInterval > 0 {
			ticker := time.NewTicker(client.keepaliveInterval)
			defer ticker.Stop()

			keepalive = ticker.C
		}

		for {
			select {
			case <-current.Done():
				if client.clear(current) {
					client.logger.Debugf("evicted disconnected SSH exec client for VM UID %s: %v",
						client.vmUID, current.Err())
				}

				return
			case <-keepalive:
				if err := current.Keepalive(); err != nil {
					if client.invalidate(current) {
						client.logger.Debugf("evicted SSH exec client for VM UID %s after keepalive failure: %v",
							client.vmUID, err)
					}

					return
				}
			}
		}
	}()
}

type sshExecClient interface {
	NewExec(options sshexec.Options) (*sshexec.Exec, error)
	Keepalive() error
	Done() <-chan struct{}
	Err() error
	Close() error
	ShouldRecreateAfter(err error) bool
}
