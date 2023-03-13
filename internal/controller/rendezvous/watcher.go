package rendezvous

import (
	"context"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/controller/rendezvous/concurrentmap"
)

var ErrNoTopic = errors.New("no watcher topic registered with this name")

type Watcher struct {
	topics *concurrentmap.ConcurrentMap[*TopicSlot]
}

type TopicSlot struct {
	ctx context.Context
	ch  chan *TopicMessage
}

type TopicMessage struct {
	Token  string
	VMUID  string
	VMPort uint16
}

func NewWatcher() *Watcher {
	return &Watcher{
		topics: concurrentmap.NewConcurrentMap[*TopicSlot](),
	}
}

func (watcher *Watcher) Subscribe(ctx context.Context, topic string) (chan *TopicMessage, func()) {
	subCtx, cancel := context.WithCancel(ctx)
	topicCh := make(chan *TopicMessage)

	watcher.topics.Store(topic, &TopicSlot{
		ctx: subCtx,
		ch:  topicCh,
	})

	return topicCh, func() {
		watcher.topics.Delete(topic)
		cancel()
	}
}

func (watcher *Watcher) Notify(ctx context.Context, topic string, msg *TopicMessage) error {
	slot, ok := watcher.topics.Load(topic)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoTopic, topic)
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
