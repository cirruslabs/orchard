package controller

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cirruslabs/orchard/internal/controller/sshexec"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeSSHExecClient struct {
	keepaliveErr error

	closeCalls     atomic.Int32
	keepaliveCalls atomic.Int32

	done      chan struct{}
	closeOnce sync.Once
}

func newFakeSSHExecClient() *fakeSSHExecClient {
	return &fakeSSHExecClient{
		done: make(chan struct{}),
	}
}

func (client *fakeSSHExecClient) NewExec(options sshexec.Options) (*sshexec.Exec, error) {
	return nil, nil
}

func (client *fakeSSHExecClient) Keepalive() error {
	client.keepaliveCalls.Add(1)

	return client.keepaliveErr
}

func (client *fakeSSHExecClient) Done() <-chan struct{} {
	return client.done
}

func (client *fakeSSHExecClient) Err() error {
	<-client.done

	return errors.New("disconnected")
}

func (client *fakeSSHExecClient) Close() error {
	client.closeCalls.Add(1)
	client.closeOnce.Do(func() {
		close(client.done)
	})

	return nil
}

func (client *fakeSSHExecClient) ShouldRecreateAfter(err error) bool {
	return true
}

func TestExecSSHClientPoolDeduplicatesConcurrentInitialization(t *testing.T) {
	pool := newExecSSHClientPool(0, zap.NewNop().Sugar())
	client := newFakeSSHExecClient()

	createStarted := make(chan struct{})
	releaseCreate := make(chan struct{})

	var createCalls atomic.Int32
	create := func() (sshExecClient, error) {
		if createCalls.Add(1) == 1 {
			close(createStarted)
		}

		<-releaseCreate

		return client, nil
	}

	const callers = 16

	start := make(chan struct{})
	errCh := make(chan error, callers)

	var wg sync.WaitGroup
	wg.Add(callers)

	for range callers {
		go func() {
			defer wg.Done()

			<-start

			_, err := pool.newExec("vm-1", sshexec.Options{}, create)
			if err != nil {
				errCh <- err
			}
		}()
	}

	close(start)
	<-createStarted
	close(releaseCreate)

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, createCalls.Load())

	pool.closeAll()
}

func TestExecSSHClientPoolKeepaliveInvalidatesClient(t *testing.T) {
	pool := newExecSSHClientPool(time.Millisecond, zap.NewNop().Sugar())
	client := newFakeSSHExecClient()
	client.keepaliveErr = errors.New("boom")

	_, err := pool.newExec("vm-1", sshexec.Options{}, func() (sshExecClient, error) {
		return client, nil
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return client.keepaliveCalls.Load() > 0 && client.closeCalls.Load() == 1
	}, time.Second, time.Millisecond)

	replacement := newFakeSSHExecClient()
	var createCalls atomic.Int32
	_, err = pool.newExec("vm-1", sshexec.Options{}, func() (sshExecClient, error) {
		createCalls.Add(1)

		return replacement, nil
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, createCalls.Load())

	pool.closeAll()
}

func TestExecSSHClientPoolWaitClearsDisconnectedClient(t *testing.T) {
	pool := newExecSSHClientPool(0, zap.NewNop().Sugar())
	client := newFakeSSHExecClient()

	_, err := pool.newExec("vm-1", sshexec.Options{}, func() (sshExecClient, error) {
		return client, nil
	})
	require.NoError(t, err)

	require.NoError(t, client.Close())

	replacement := newFakeSSHExecClient()
	var createCalls atomic.Int32
	require.Eventually(t, func() bool {
		_, err := pool.newExec("vm-1", sshexec.Options{}, func() (sshExecClient, error) {
			createCalls.Add(1)

			return replacement, nil
		})
		require.NoError(t, err)

		return createCalls.Load() == 1
	}, time.Second, time.Millisecond)

	pool.closeAll()
}
