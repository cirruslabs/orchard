package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cirruslabs/orchard/pkg/resource/v1"
)

const imagePullJobsEndpointPrefix = "imagepulljobs"

type ImagePullJobsService struct {
	client *Client
}

func (service *ImagePullJobsService) Create(ctx context.Context, pullJob *v1.ImagePullJob) error {
	err := service.client.request(ctx, http.MethodPost, imagePullJobsEndpointPrefix, pullJob, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (service *ImagePullJobsService) List(ctx context.Context) ([]v1.ImagePullJob, error) {
	var pullJobs []v1.ImagePullJob

	err := service.client.request(ctx, http.MethodGet, imagePullJobsEndpointPrefix, nil, &pullJobs, nil)
	if err != nil {
		return nil, err
	}

	return pullJobs, nil
}

func (service *ImagePullJobsService) Get(ctx context.Context, name string) (*v1.ImagePullJob, error) {
	var pullJob v1.ImagePullJob

	err := service.client.request(ctx, http.MethodGet, fmt.Sprintf("%s/%s", imagePullJobsEndpointPrefix,
		url.PathEscape(name)), nil, &pullJob, nil)
	if err != nil {
		return nil, err
	}

	return &pullJob, nil
}

func (service *ImagePullJobsService) Update(ctx context.Context, pull v1.ImagePullJob) (*v1.ImagePullJob, error) {
	var updatedPullJob v1.ImagePullJob

	err := service.client.request(ctx, http.MethodPut, fmt.Sprintf("%s/%s", imagePullJobsEndpointPrefix,
		url.PathEscape(pull.Name)), pull, &updatedPullJob, nil)
	if err != nil {
		return &updatedPullJob, err
	}

	return &updatedPullJob, nil
}

func (service *ImagePullJobsService) UpdateState(ctx context.Context, pull v1.ImagePullJob) (*v1.ImagePullJob, error) {
	var updatedPullJob v1.ImagePullJob

	err := service.client.request(ctx, http.MethodPut, fmt.Sprintf("%s/%s/state", imagePullJobsEndpointPrefix,
		url.PathEscape(pull.Name)), pull, &updatedPullJob, nil)
	if err != nil {
		return &updatedPullJob, err
	}

	return &updatedPullJob, nil
}

func (service *ImagePullJobsService) Delete(ctx context.Context, name string) error {
	err := service.client.request(ctx, http.MethodDelete, fmt.Sprintf("%s/%s", imagePullJobsEndpointPrefix,
		url.PathEscape(name)), nil, nil, nil)
	if err != nil {
		return err
	}

	return nil
}
