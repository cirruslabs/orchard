package notifier_test

import (
	"context"
	"fmt"
	"github.com/cirruslabs/orchard/internal/controller/notifier"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestNotifier(t *testing.T) {
	ctx := context.Background()

	notifier := notifier.NewNotifier()

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
