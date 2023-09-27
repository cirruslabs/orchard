package notifier

import (
	"context"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/concurrentmap"
	"github.com/cirruslabs/orchard/rpc"
	"go.uber.org/zap"
)

var ErrNoWorker = errors.New("no worker registered with this name")

type Notifier struct {
	workers *concurrentmap.ConcurrentMap[*WorkerSlot]
	logger  *zap.SugaredLogger
}

type WorkerSlot struct {
	ctx context.Context
	ch  chan *rpc.WatchInstruction
}

func NewNotifier(logger *zap.SugaredLogger) *Notifier {
	return &Notifier{
		workers: concurrentmap.NewConcurrentMap[*WorkerSlot](),
		logger:  logger,
	}
}

func (watcher *Notifier) Register(ctx context.Context, worker string) (chan *rpc.WatchInstruction, func()) {
	subCtx, cancel := context.WithCancel(ctx)
	workerCh := make(chan *rpc.WatchInstruction)

	watcher.logger.Debugf("registering worker %s", worker)
	watcher.workers.Store(worker, &WorkerSlot{
		ctx: subCtx,
		ch:  workerCh,
	})

	return workerCh, func() {
		watcher.logger.Debugf("deleting worker %s", worker)
		watcher.workers.Delete(worker)
		cancel()
	}
}

func (watcher *Notifier) Notify(ctx context.Context, worker string, msg *rpc.WatchInstruction) error {
	slot, ok := watcher.workers.Load(worker)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoWorker, worker)
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
