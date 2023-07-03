package client

import (
	"context"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"net/http"
)

type ClusterSettingsService struct {
	client *Client
}

func (service *ClusterSettingsService) Get(ctx context.Context) (*v1.ClusterSettings, error) {
	var clusterSettings v1.ClusterSettings

	err := service.client.request(ctx, http.MethodGet, "cluster-settings", nil, &clusterSettings, nil)
	if err != nil {
		return nil, err
	}

	return &clusterSettings, nil
}

func (service *ClusterSettingsService) Set(ctx context.Context, clusterSettings *v1.ClusterSettings) error {
	return service.client.request(ctx, http.MethodPut, "cluster-settings", clusterSettings, nil, nil)
}
