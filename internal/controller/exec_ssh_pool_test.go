package controller

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cirruslabs/orchard/internal/controller/sshexec"
	"github.com/stretchr/testify/require"
)

type fakeExecSSHTransport struct {
	newExec func(sshexec.Options) (sshExecRunner, error)

	closeCalls atomic.Int32
}

func (transport *fakeExecSSHTransport) NewExec(options sshexec.Options) (sshExecRunner, error) {
	if transport.newExec != nil {
		return transport.newExec(options)
	}

	return &fakeExec{}, nil
}

func (transport *fakeExecSSHTransport) Close() error {
	transport.closeCalls.Add(1)

	return nil
}

func TestExecSSHTransportCacheConcurrentGetOrCreateReusesOneTransport(t *testing.T) {
	cache := newExecSSHTransportCache()
	key := execSSHTransportKey{workerName: "worker", vmUID: "vm", restartCount: 1}

	createStarted := make(chan struct{})
	releaseCreate := make(chan struct{})
	var createCalls atomic.Int32
	transport := &fakeExecSSHTransport{}

	create := func() (execSSHTransport, error) {
		if createCalls.Add(1) == 1 {
			close(createStarted)
		}
		<-releaseCreate

		return transport, nil
	}

	const callersCount = 16
	transports := make([]execSSHTransport, callersCount)
	reused := make([]bool, callersCount)
	errCh := make(chan error, callersCount)

	var wg sync.WaitGroup
	wg.Add(callersCount)
	for i := range callersCount {
		go func() {
			defer wg.Done()

			var err error
			transports[i], reused[i], err = cache.getOrCreate(key, create)
			if err != nil {
				errCh <- err
			}
		}()
	}

	<-createStarted
	close(releaseCreate)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, createCalls.Load())

	for _, cachedTransport := range transports {
		require.Same(t, transport, cachedTransport)
	}
	require.Equal(t, callersCount-1, countTrue(reused))
}

func TestExecSSHTransportCacheSeparatesVMIncarnations(t *testing.T) {
	cache := newExecSSHTransportCache()
	var createCalls atomic.Int32

	create := func() (execSSHTransport, error) {
		createCalls.Add(1)

		return &fakeExecSSHTransport{}, nil
	}

	firstTransport, _, err := cache.getOrCreate(
		execSSHTransportKey{workerName: "worker", vmUID: "vm", restartCount: 1}, create)
	require.NoError(t, err)
	secondTransport, _, err := cache.getOrCreate(
		execSSHTransportKey{workerName: "worker", vmUID: "vm", restartCount: 2}, create)
	require.NoError(t, err)

	require.EqualValues(t, 2, createCalls.Load())
	require.NotSame(t, firstTransport, secondTransport)
}

func TestExecSSHTransportCacheDiscardClosesExpectedTransport(t *testing.T) {
	cache := newExecSSHTransportCache()
	key := execSSHTransportKey{workerName: "worker", vmUID: "vm", restartCount: 1}
	transport := &fakeExecSSHTransport{}

	cachedTransport, _, err := cache.getOrCreate(key, func() (execSSHTransport, error) {
		return transport, nil
	})
	require.NoError(t, err)

	cache.discard(key, cachedTransport)
	require.EqualValues(t, 1, transport.closeCalls.Load())
	require.Empty(t, cache.entries)
}

func TestExecSSHTransportCacheDiscardIgnoresReplacedTransport(t *testing.T) {
	cache := newExecSSHTransportCache()
	key := execSSHTransportKey{workerName: "worker", vmUID: "vm", restartCount: 1}
	originalTransport := &fakeExecSSHTransport{}
	replacementTransport := &fakeExecSSHTransport{}

	cachedTransport, _, err := cache.getOrCreate(key, func() (execSSHTransport, error) {
		return originalTransport, nil
	})
	require.NoError(t, err)

	cache.entries[key] = replacementTransport
	cache.discard(key, cachedTransport)

	require.EqualValues(t, 0, originalTransport.closeCalls.Load())
	require.Same(t, replacementTransport, cache.entries[key])
}

func TestExecSSHTransportCacheKeepsTransportAcrossExecs(t *testing.T) {
	cache := newExecSSHTransportCache()
	key := execSSHTransportKey{workerName: "worker", vmUID: "vm", restartCount: 1}
	transport := &fakeExecSSHTransport{
		newExec: func(sshexec.Options) (sshExecRunner, error) {
			return nil, errors.New("failed to open channel")
		},
	}
	var createCalls atomic.Int32

	create := func() (execSSHTransport, error) {
		createCalls.Add(1)

		return transport, nil
	}

	firstTransport, reused, err := cache.getOrCreate(key, create)
	require.NoError(t, err)
	require.False(t, reused)
	secondTransport, reused, err := cache.getOrCreate(key, create)
	require.NoError(t, err)
	require.True(t, reused)

	require.Same(t, firstTransport, secondTransport)
	require.EqualValues(t, 1, createCalls.Load())
	require.EqualValues(t, 0, transport.closeCalls.Load())
}

func TestExecSSHTransportCacheCloseAllClosesTransports(t *testing.T) {
	cache := newExecSSHTransportCache()
	transport := &fakeExecSSHTransport{}

	_, _, err := cache.getOrCreate(
		execSSHTransportKey{workerName: "worker", vmUID: "vm", restartCount: 1},
		func() (execSSHTransport, error) {
			return transport, nil
		})
	require.NoError(t, err)

	cache.closeAll()
	require.EqualValues(t, 1, transport.closeCalls.Load())
	require.Empty(t, cache.entries)
}

func countTrue(values []bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}

	return count
}
