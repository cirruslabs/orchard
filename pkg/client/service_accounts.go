package client

import (
	"context"
	"fmt"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"net/http"
	"net/url"
)

type ServiceAccountsService struct {
	client *Client
}

func (service *ServiceAccountsService) Create(ctx context.Context, serviceAccount *v1.ServiceAccount) error {
	err := service.client.request(ctx, http.MethodPost, "service-accounts",
		serviceAccount, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (service *ServiceAccountsService) List(ctx context.Context) ([]v1.ServiceAccount, error) {
	var serviceAccounts []v1.ServiceAccount

	err := service.client.request(ctx, http.MethodGet, "service-accounts",
		nil, &serviceAccounts, nil)
	if err != nil {
		return nil, err
	}

	return serviceAccounts, nil
}

func (service *ServiceAccountsService) Get(ctx context.Context, name string) (*v1.ServiceAccount, error) {
	var serviceAccount v1.ServiceAccount

	err := service.client.request(ctx, http.MethodGet,
		fmt.Sprintf("service-accounts/%s", url.PathEscape(name)),
		nil, &serviceAccount, nil)
	if err != nil {
		return nil, err
	}

	return &serviceAccount, nil
}

func (service *ServiceAccountsService) Update(ctx context.Context, serviceAccount *v1.ServiceAccount) error {
	err := service.client.request(ctx, http.MethodPut,
		fmt.Sprintf("service-accounts/%s", url.PathEscape(serviceAccount.Name)),
		serviceAccount, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (service *ServiceAccountsService) Delete(ctx context.Context, name string, force bool) error {
	params := map[string]string{}

	if force {
		params["force"] = "true"
	}

	err := service.client.request(ctx, http.MethodDelete,
		fmt.Sprintf("service-accounts/%s", url.PathEscape(name)),
		nil, nil, params)
	if err != nil {
		return err
	}

	return nil
}
