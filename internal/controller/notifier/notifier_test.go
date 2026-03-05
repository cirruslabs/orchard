package notifier_test

import (
	"context"
	"fmt"
	"github.com/cirruslabs/orchard/internal/controller/notifier"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"sync"
	"testing"
	"time"
)

func TestNotifier(t *testing.T) {
	ctx := context.Background()

	notifier := notifier.NewNotifier(zap.NewNop().Sugar())

	var topic = uuid.New().String()

	msgCh, cancel := notifier.Register(context.Background(), topic)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		require.NoError(t, notifier.Notify(ctx, topic, nil))

		time.Sleep(time.Second)

		require.NoError(t, notifier.Notify(ctx, topic, nil))

		wg.Done()
	}()

	fmt.Println("waiting for the message...")

	<-msgCh

	fmt.Println("received first message")

	<-msgCh

	fmt.Println("received second message")

	wg.Wait()
}

func TestNotifierReRegisterKeepsNewestSlot(t *testing.T) {
	ctx := context.Background()
	watcher := notifier.NewNotifier(zap.NewNop().Sugar())

	const worker = "worker-a"

	_, staleCancel := watcher.Register(ctx, worker)
	newestCh, newestCancel := watcher.Register(ctx, worker)
	defer newestCancel()

	// Simulate stale connection cleanup arriving after the worker has already re-registered.
	staleCancel()

	notifyCtx, notifyCancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer notifyCancel()

	notifyErrCh := make(chan error, 1)
	go func() {
		notifyErrCh <- watcher.Notify(notifyCtx, worker, nil)
	}()

	select {
	case <-newestCh:
	case err := <-notifyErrCh:
		require.NoError(t, err)
		t.Fatal("notify returned before delivering message to newest registration")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notify delivery")
	}

	require.NoError(t, <-notifyErrCh)
}
