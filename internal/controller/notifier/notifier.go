package notifier

import (
	"context"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/concurrentmap"
	"github.com/cirruslabs/orchard/rpc"
)

var ErrNoWorker = errors.New("no worker registered with this name")

type Notifier struct {
	workers *concurrentmap.ConcurrentMap[*WorkerSlot]
}

type WorkerSlot struct {
	ctx context.Context
	ch  chan *rpc.WatchInstruction
}

func NewNotifier() *Notifier {
	return &Notifier{
		workers: concurrentmap.NewConcurrentMap[*WorkerSlot](),
	}
}

func (watcher *Notifier) Register(ctx context.Context, workerUID string) (chan *rpc.WatchInstruction, func()) {
	subCtx, cancel := context.WithCancel(ctx)
	workerCh := make(chan *rpc.WatchInstruction)

	watcher.workers.Store(workerUID, &WorkerSlot{
		ctx: subCtx,
		ch:  workerCh,
	})

	return workerCh, func() {
		watcher.workers.Delete(workerUID)
		cancel()
	}
}

func (watcher *Notifier) Notify(ctx context.Context, workerUID string, msg *rpc.WatchInstruction) error {
	slot, ok := watcher.workers.Load(workerUID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoWorker, workerUID)
	}

	select {
	case slot.ch <- msg:
		return nil
	case <-slot.ctx.Done():
		return slot.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}
