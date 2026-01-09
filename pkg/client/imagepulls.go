package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cirruslabs/orchard/pkg/resource/v1"
)

const pullsEndpointPrefix = "imagepulls"

type ImagePullsService struct {
	client *Client
}

func (service *ImagePullsService) Create(ctx context.Context, pull *v1.ImagePull) error {
	err := service.client.request(ctx, http.MethodPost, pullsEndpointPrefix, pull, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (service *ImagePullsService) FindForWorker(ctx context.Context, worker string) ([]v1.ImagePull, error) {
	allPulls, err := service.List(ctx)
	if err != nil {
		return nil, err
	}

	var result []v1.ImagePull

	for _, pull := range allPulls {
		if pull.Worker != worker {
			continue
		}

		result = append(result, pull)
	}

	return result, nil
}

func (service *ImagePullsService) List(ctx context.Context) ([]v1.ImagePull, error) {
	var pulls []v1.ImagePull

	err := service.client.request(ctx, http.MethodGet, pullsEndpointPrefix, nil, &pulls, nil)
	if err != nil {
		return nil, err
	}

	return pulls, nil
}

func (service *ImagePullsService) Get(ctx context.Context, name string) (*v1.ImagePull, error) {
	var pull v1.ImagePull

	err := service.client.request(ctx, http.MethodGet, fmt.Sprintf("%s/%s", pullsEndpointPrefix,
		url.PathEscape(name)), nil, &pull, nil)
	if err != nil {
		return nil, err
	}

	return &pull, nil
}

func (service *ImagePullsService) Update(ctx context.Context, pull v1.ImagePull) (*v1.ImagePull, error) {
	var updatedPull v1.ImagePull

	err := service.client.request(ctx, http.MethodPut, fmt.Sprintf("%s/%s", pullsEndpointPrefix,
		url.PathEscape(pull.Name)), pull, &updatedPull, nil)
	if err != nil {
		return &updatedPull, err
	}

	return &updatedPull, nil
}

func (service *ImagePullsService) UpdateState(ctx context.Context, pull v1.ImagePull) (*v1.ImagePull, error) {
	var updatedPull v1.ImagePull

	err := service.client.request(ctx, http.MethodPut, fmt.Sprintf("%s/%s/state", pullsEndpointPrefix,
		url.PathEscape(pull.Name)), pull, &updatedPull, nil)
	if err != nil {
		return &updatedPull, err
	}

	return &updatedPull, nil
}

func (service *ImagePullsService) Delete(ctx context.Context, name string) error {
	err := service.client.request(ctx, http.MethodDelete, fmt.Sprintf("%s/%s", pullsEndpointPrefix,
		url.PathEscape(name)), nil, nil, nil)
	if err != nil {
		return err
	}

	return nil
}
