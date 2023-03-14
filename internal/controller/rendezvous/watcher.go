package rendezvous

import (
	"context"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/controller/rendezvous/concurrentmap"
	"github.com/cirruslabs/orchard/rpc"
)

var ErrNoWorker = errors.New("no worker registered with this name")

type Watcher struct {
	workers *concurrentmap.ConcurrentMap[*WorkerSlot]
}

type WorkerSlot struct {
	ctx context.Context
	ch  chan *rpc.WatchFromController
}

func NewWatcher() *Watcher {
	return &Watcher{
		workers: concurrentmap.NewConcurrentMap[*WorkerSlot](),
	}
}

func (watcher *Watcher) Subscribe(ctx context.Context, worker string) (chan *rpc.WatchFromController, func()) {
	subCtx, cancel := context.WithCancel(ctx)
	workerCh := make(chan *rpc.WatchFromController)

	watcher.workers.Store(worker, &WorkerSlot{
		ctx: subCtx,
		ch:  workerCh,
	})

	return workerCh, func() {
		watcher.workers.Delete(worker)
		cancel()
	}
}

func (watcher *Watcher) Notify(ctx context.Context, topic string, msg *rpc.WatchFromController) error {
	slot, ok := watcher.workers.Load(topic)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoWorker, topic)
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
