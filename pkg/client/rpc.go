package client

import (
	"context"
	"encoding/json"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"net"
	"net/http"
)

type RPCService struct {
	client *Client
}

func (service *RPCService) Watch(
	ctx context.Context,
	workerName string,
) (<-chan *v1.WatchInstruction, <-chan error, error) {
	wsConn, err := service.client.wsRequest(ctx, "rpc/watch", map[string]string{
		"workerName": workerName,
	})
	if err != nil {
		return nil, nil, err
	}

	watchEventCh := make(chan *v1.WatchInstruction)
	errCh := make(chan error, 1)

	go func() {
		defer func() {
			_ = wsConn.Close()
		}()

		decoder := json.NewDecoder(wsConn)

		for {
			var watchInstruction v1.WatchInstruction

			if err := decoder.Decode(&watchInstruction); err != nil {
				errCh <- err

				return
			}

			watchEventCh <- &watchInstruction
		}
	}()

	return watchEventCh, errCh, nil
}

func (service *RPCService) RespondPortForward(
	ctx context.Context,
	session string,
	errorMessage string,
) (net.Conn, error) {
	return service.client.wsRequest(ctx, "rpc/port-forward", map[string]string{
		"session":      session,
		"errorMessage": errorMessage,
	})
}

func (service *RPCService) RespondIP(
	ctx context.Context,
	session string,
	ip string,
	errorMessage string,
) error {
	return service.client.request(ctx, http.MethodPost, "rpc/resolve-ip", nil, nil, map[string]string{
		"session":      session,
		"ip":           ip,
		"errorMessage": errorMessage,
	})
}
