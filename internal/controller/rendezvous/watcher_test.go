package rendezvous_test

import (
	"context"
	"fmt"
	"github.com/cirruslabs/orchard/internal/controller/rendezvous"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestWatcher(t *testing.T) {
	ctx := context.Background()

	watcher := rendezvous.NewWatcher()

	var topic = uuid.New().String()

	msgCh, cancel := watcher.Subscribe(context.Background(), topic)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		require.NoError(t, watcher.Notify(ctx, topic, &rendezvous.WorkerMessage{}))

		time.Sleep(time.Second)

		require.NoError(t, watcher.Notify(ctx, topic, &rendezvous.WorkerMessage{}))

		wg.Done()
	}()

	fmt.Println("waiting for the message...")

	<-msgCh

	fmt.Println("received first message")

	<-msgCh

	fmt.Println("received second message")

	wg.Wait()
}
