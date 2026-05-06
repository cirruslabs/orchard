package controller

import (
	"context"
	"sync"

	"github.com/cirruslabs/orchard/internal/controller/sshexec"
)

type execSSHTransport interface {
	NewExec(options sshexec.Options) (sshExecRunner, error)
	Close() error
}

type execSSHClientTransport struct {
	client *sshexec.Client
}

func (transport *execSSHClientTransport) NewExec(options sshexec.Options) (sshExecRunner, error) {
	return transport.client.NewExec(options)
}

func (transport *execSSHClientTransport) Close() error {
	return transport.client.Close()
}

type execSSHTransportKey struct {
	workerName   string
	vmUID        string
	restartCount uint64
}

type execSSHTransportEntry struct {
	key       execSSHTransportKey
	transport execSSHTransport
	refs      int
	closed    bool
}

type execSSHTransportCreation struct {
	done chan struct{}
	err  error
}

type execSSHTransportPool struct {
	mu       sync.Mutex
	entries  map[execSSHTransportKey]*execSSHTransportEntry
	creating map[execSSHTransportKey]*execSSHTransportCreation
}

func newExecSSHTransportPool() *execSSHTransportPool {
	return &execSSHTransportPool{
		entries:  map[execSSHTransportKey]*execSSHTransportEntry{},
		creating: map[execSSHTransportKey]*execSSHTransportCreation{},
	}
}

func (pool *execSSHTransportPool) acquire(
	ctx context.Context,
	key execSSHTransportKey,
	create func() (execSSHTransport, error),
) (*execSSHTransportLease, error) {
	for {
		pool.mu.Lock()

		if entry, ok := pool.entries[key]; ok {
			entry.refs++
			pool.mu.Unlock()

			return &execSSHTransportLease{
				pool:   pool,
				entry:  entry,
				reused: true,
			}, nil
		}

		if creation, ok := pool.creating[key]; ok {
			pool.mu.Unlock()

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-creation.done:
				if creation.err != nil {
					return nil, creation.err
				}
			}

			continue
		}

		creation := &execSSHTransportCreation{done: make(chan struct{})}
		pool.creating[key] = creation
		pool.mu.Unlock()

		transport, err := create()

		pool.mu.Lock()
		delete(pool.creating, key)
		creation.err = err

		var entry *execSSHTransportEntry
		if err == nil {
			entry = &execSSHTransportEntry{
				key:       key,
				transport: transport,
				refs:      1,
			}
			pool.entries[key] = entry
		}

		close(creation.done)
		pool.mu.Unlock()

		if err != nil {
			return nil, err
		}

		return &execSSHTransportLease{
			pool:  pool,
			entry: entry,
		}, nil
	}
}

func (pool *execSSHTransportPool) release(entry *execSSHTransportEntry) {
	var transport execSSHTransport

	pool.mu.Lock()
	if entry.closed {
		pool.mu.Unlock()

		return
	}

	entry.refs--
	if entry.refs == 0 {
		entry.closed = true
		if pool.entries[entry.key] == entry {
			delete(pool.entries, entry.key)
		}
		transport = entry.transport
	}
	pool.mu.Unlock()

	if transport != nil {
		_ = transport.Close()
	}
}

func (pool *execSSHTransportPool) closeAll() {
	pool.mu.Lock()
	entries := make([]*execSSHTransportEntry, 0, len(pool.entries))
	for key, entry := range pool.entries {
		entry.closed = true
		delete(pool.entries, key)
		entries = append(entries, entry)
	}
	pool.mu.Unlock()

	for _, entry := range entries {
		_ = entry.transport.Close()
	}
}

type execSSHTransportLease struct {
	pool   *execSSHTransportPool
	entry  *execSSHTransportEntry
	reused bool

	releaseOnce sync.Once
}

func (lease *execSSHTransportLease) transport() execSSHTransport {
	return lease.entry.transport
}

func (lease *execSSHTransportLease) release() {
	lease.releaseOnce.Do(func() {
		lease.pool.release(lease.entry)
	})
}
