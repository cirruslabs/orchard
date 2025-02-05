package notifier

import (
	"context"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/concurrentmap"
	"github.com/cirruslabs/orchard/rpc"
	"go.uber.org/zap"
	"time"
)

var ErrNoWorker = errors.New("no worker registered with this name")

type Notifier struct {
	workers *concurrentmap.ConcurrentMap[*WorkerSlot]
	logger  *zap.SugaredLogger
	// Add wait duration before reporting worker not found
	workerWaitTimeout time.Duration
}

type WorkerSlot struct {
	ctx context.Context
	ch  chan *rpc.WatchInstruction
}

func NewNotifier(logger *zap.SugaredLogger) *Notifier {
	return &Notifier{
		workers:           concurrentmap.NewConcurrentMap[*WorkerSlot](),
		logger:            logger,
		workerWaitTimeout: 30 * time.Second, // Default timeout of 30 seconds
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

	deadline := time.Now().Add(watcher.workerWaitTimeout)
	for !ok && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
			watcher.logger.Infof("waiting for worker %s to re-connect...", worker)
		}
		slot, ok = watcher.workers.Load(worker)
	}
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
