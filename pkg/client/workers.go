package client

import (
	"context"
	"fmt"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"net/http"
)

type WorkersService struct {
	client *Client
}

func (service *WorkersService) Create(ctx context.Context, worker *v1.Worker) (*v1.Worker, error) {
	err := service.client.request(ctx, http.MethodPost, "workers",
		worker, &worker, nil)
	if err != nil {
		return nil, err
	}

	return worker, nil
}

func (service *WorkersService) List(ctx context.Context) ([]v1.Worker, error) {
	var workers []v1.Worker

	err := service.client.request(ctx, http.MethodGet, "workers",
		nil, &workers, nil)
	if err != nil {
		return nil, err
	}

	return workers, nil
}

func (service *WorkersService) Get(ctx context.Context, name string) (*v1.Worker, error) {
	var worker v1.Worker

	err := service.client.request(ctx, http.MethodGet, fmt.Sprintf("workers/%s", name),
		nil, &worker, nil)
	if err != nil {
		return nil, err
	}

	return &worker, nil
}

func (service *WorkersService) Update(ctx context.Context, worker *v1.Worker) error {
	err := service.client.request(ctx, http.MethodPut, fmt.Sprintf("workers/%s", worker.Name),
		worker, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (service *WorkersService) Delete(ctx context.Context, name string) error {
	err := service.client.request(ctx, http.MethodDelete, fmt.Sprintf("workers/%s", name),
		nil, nil, nil)
	if err != nil {
		return err
	}

	return nil
}
