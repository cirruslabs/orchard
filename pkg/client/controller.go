package client

import (
	"context"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"net/http"
)

type ControllerService struct {
	client *Client
}

func (service *ControllerService) Info(ctx context.Context) (v1.ControllerInfo, error) {
	var controllerInfo v1.ControllerInfo

	err := service.client.request(ctx, http.MethodGet, "controller/info", nil, &controllerInfo,
		nil)
	if err != nil {
		return controllerInfo, err
	}

	return controllerInfo, nil
}
