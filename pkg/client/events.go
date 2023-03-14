package client

import (
	"context"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/hashicorp/go-multierror"
	"io"
	"net/http"
)

type EventStreamer struct {
	client        *Client
	endpoint      string
	eventsChannel chan v1.Event
	sendErr       error
	io.Closer
}

func NewEventStreamer(client *Client, endpoint string) *EventStreamer {
	streamer := &EventStreamer{
		client:        client,
		endpoint:      endpoint,
		eventsChannel: make(chan v1.Event),
	}
	go streamer.stream()
	return streamer
}

func (streamer *EventStreamer) Stream(event v1.Event) {
	streamer.eventsChannel <- event
}

func (streamer *EventStreamer) stream() {
	ctx := context.Background()

	for {
		events, finished := streamer.readAvailableEvents()
		err := streamer.client.request(ctx, http.MethodPost, streamer.endpoint, events, nil, nil)
		if err != nil {
			streamer.sendErr = multierror.Append(streamer.sendErr, err)
		}
		if finished {
			break
		}
	}
}

func (streamer *EventStreamer) readAvailableEvents() ([]v1.Event, bool) {
	// blocking wait for at least one event
	result := []v1.Event{<-streamer.eventsChannel}

	for {
		select {
		case nextEvent, more := <-streamer.eventsChannel:
			result = append(result, nextEvent)
			if !more {
				return result, true
			}
		default:
			return result, false
		}
	}
}

func (streamer *EventStreamer) Close() error {
	close(streamer.eventsChannel)
	return streamer.sendErr
}
