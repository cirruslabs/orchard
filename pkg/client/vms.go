package client

import (
	"context"
	"fmt"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"net/http"
)

type VMsService struct {
	client *Client
}

func (service *VMsService) Create(ctx context.Context, vm *v1.VM) error {
	err := service.client.request(ctx, http.MethodPost, "vms",
		vm, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (service *VMsService) List(ctx context.Context) ([]v1.VM, error) {
	var vms []v1.VM

	err := service.client.request(ctx, http.MethodGet, "vms",
		nil, &vms, nil)
	if err != nil {
		return nil, err
	}

	return vms, nil
}

func (service *VMsService) Get(ctx context.Context, name string) (*v1.VM, error) {
	var vm v1.VM

	err := service.client.request(ctx, http.MethodGet, fmt.Sprintf("vms/%s", name),
		nil, &vm, nil)
	if err != nil {
		return nil, err
	}

	return &vm, nil
}

func (service *VMsService) Update(ctx context.Context, vm *v1.VM) error {
	err := service.client.request(ctx, http.MethodPut, fmt.Sprintf("vms/%s", vm.Name),
		vm, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (service *VMsService) Delete(ctx context.Context, name string, force bool) error {
	params := map[string]string{}

	if force {
		params["force"] = "true"
	}

	err := service.client.request(ctx, http.MethodDelete, fmt.Sprintf("vms/%s", name),
		nil, nil, params)
	if err != nil {
		return err
	}

	return nil
}
