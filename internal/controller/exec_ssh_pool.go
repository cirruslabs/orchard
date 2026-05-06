package controller

import (
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

type execSSHTransportCache struct {
	mu      sync.Mutex
	entries map[execSSHTransportKey]execSSHTransport
}

func newExecSSHTransportCache() *execSSHTransportCache {
	return &execSSHTransportCache{
		entries: map[execSSHTransportKey]execSSHTransport{},
	}
}

func (cache *execSSHTransportCache) getOrCreate(
	key execSSHTransportKey,
	create func() (execSSHTransport, error),
) (execSSHTransport, bool, error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if transport, ok := cache.entries[key]; ok {
		return transport, true, nil
	}

	transport, err := create()
	if err != nil {
		return nil, false, err
	}

	cache.entries[key] = transport

	return transport, false, nil
}

func (cache *execSSHTransportCache) discard(key execSSHTransportKey, expected execSSHTransport) {
	var transport execSSHTransport

	cache.mu.Lock()
	if cache.entries[key] == expected {
		transport = expected
		delete(cache.entries, key)
	}
	cache.mu.Unlock()

	if transport != nil {
		_ = transport.Close()
	}
}

func (cache *execSSHTransportCache) closeAll() {
	cache.mu.Lock()
	transports := make([]execSSHTransport, 0, len(cache.entries))
	for key, transport := range cache.entries {
		delete(cache.entries, key)
		transports = append(transports, transport)
	}
	cache.mu.Unlock()

	for _, transport := range transports {
		_ = transport.Close()
	}
}
