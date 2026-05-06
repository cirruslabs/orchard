package controller

import (
	"context"
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

func TestExecSSHTransportPoolConcurrentAcquireReusesOneTransport(t *testing.T) {
	pool := newExecSSHTransportPool()
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

	const leasesCount = 16
	leases := make([]*execSSHTransportLease, leasesCount)
	errCh := make(chan error, leasesCount)

	var wg sync.WaitGroup
	wg.Add(leasesCount)
	for i := range leasesCount {
		go func() {
			defer wg.Done()

			lease, err := pool.acquire(context.Background(), key, create)
			if err != nil {
				errCh <- err

				return
			}

			leases[i] = lease
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

	for _, lease := range leases {
		require.NotNil(t, lease)
		require.Same(t, transport, lease.transport())
		lease.release()
	}
}

func TestExecSSHTransportPoolClosesOnLastRelease(t *testing.T) {
	pool := newExecSSHTransportPool()
	key := execSSHTransportKey{workerName: "worker", vmUID: "vm", restartCount: 1}
	transport := &fakeExecSSHTransport{}

	create := func() (execSSHTransport, error) {
		return transport, nil
	}

	firstLease, err := pool.acquire(context.Background(), key, create)
	require.NoError(t, err)
	secondLease, err := pool.acquire(context.Background(), key, create)
	require.NoError(t, err)

	firstLease.release()
	require.EqualValues(t, 0, transport.closeCalls.Load())

	secondLease.release()
	require.EqualValues(t, 1, transport.closeCalls.Load())
	require.Empty(t, pool.entries)
}

func TestExecSSHTransportPoolSeparatesVMIncarnations(t *testing.T) {
	pool := newExecSSHTransportPool()
	var createCalls atomic.Int32

	create := func() (execSSHTransport, error) {
		createCalls.Add(1)

		return &fakeExecSSHTransport{}, nil
	}

	firstLease, err := pool.acquire(context.Background(),
		execSSHTransportKey{workerName: "worker", vmUID: "vm", restartCount: 1}, create)
	require.NoError(t, err)
	secondLease, err := pool.acquire(context.Background(),
		execSSHTransportKey{workerName: "worker", vmUID: "vm", restartCount: 2}, create)
	require.NoError(t, err)
	defer firstLease.release()
	defer secondLease.release()

	require.EqualValues(t, 2, createCalls.Load())
	require.NotSame(t, firstLease.transport(), secondLease.transport())
}

func TestExecSSHTransportPoolKeepsSharedTransportAfterSessionCreationFailure(t *testing.T) {
	pool := newExecSSHTransportPool()
	key := execSSHTransportKey{workerName: "worker", vmUID: "vm", restartCount: 1}
	var createCalls atomic.Int32
	transport := &fakeExecSSHTransport{
		newExec: func(sshexec.Options) (sshExecRunner, error) {
			return nil, errors.New("failed to open channel")
		},
	}

	create := func() (execSSHTransport, error) {
		createCalls.Add(1)

		return transport, nil
	}

	activeLease, err := pool.acquire(context.Background(), key, create)
	require.NoError(t, err)
	failedLease, err := pool.acquire(context.Background(), key, create)
	require.NoError(t, err)
	require.True(t, failedLease.reused)

	_, err = failedLease.transport().NewExec(sshexec.Options{})
	require.ErrorContains(t, err, "failed to open channel")
	failedLease.release()

	require.EqualValues(t, 1, createCalls.Load())
	require.EqualValues(t, 0, transport.closeCalls.Load())
	require.Len(t, pool.entries, 1)

	activeLease.release()
	require.EqualValues(t, 1, transport.closeCalls.Load())
}

func TestExecSSHTransportPoolCloseAllClosesActiveTransports(t *testing.T) {
	pool := newExecSSHTransportPool()
	transport := &fakeExecSSHTransport{}

	lease, err := pool.acquire(context.Background(),
		execSSHTransportKey{workerName: "worker", vmUID: "vm", restartCount: 1},
		func() (execSSHTransport, error) {
			return transport, nil
		})
	require.NoError(t, err)

	pool.closeAll()
	require.EqualValues(t, 1, transport.closeCalls.Load())
	require.Empty(t, pool.entries)

	lease.release()
	require.EqualValues(t, 1, transport.closeCalls.Load())
}
