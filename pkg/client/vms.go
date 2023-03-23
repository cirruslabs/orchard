package client

import (
	"context"
	"fmt"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"golang.org/x/net/websocket"
	"net/http"
	"strconv"
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

func (service *VMsService) FindForWorker(ctx context.Context, worker string) (map[string]v1.VM, error) {
	allVms, err := service.List(ctx)
	if err != nil {
		return nil, err
	}

	var filteredVms = make(map[string]v1.VM)

	for _, vmResource := range allVms {
		if vmResource.Worker != worker {
			continue
		}

		filteredVms[vmResource.UID] = vmResource
	}

	return filteredVms, nil
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

func (service *VMsService) Stop(ctx context.Context, name string) (*v1.VM, error) {
	var vm v1.VM

	err := service.client.request(ctx, http.MethodGet, fmt.Sprintf("vms/%s", name),
		nil, &vm, nil)
	if err != nil {
		return nil, err
	}

	vm.Status = v1.VMStatusStopping

	return service.Update(ctx, vm)
}

func (service *VMsService) Update(ctx context.Context, vm v1.VM) (*v1.VM, error) {
	var updatedVM v1.VM
	err := service.client.request(ctx, http.MethodPut, fmt.Sprintf("vms/%s", vm.Name),
		vm, &updatedVM, nil)
	if err != nil {
		return &updatedVM, err
	}

	return &updatedVM, nil
}

func (service *VMsService) Delete(ctx context.Context, name string) error {
	err := service.client.request(ctx, http.MethodDelete, fmt.Sprintf("vms/%s", name),
		nil, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (service *VMsService) PortForward(ctx context.Context, name string, port uint16) (*websocket.Conn, error) {
	return service.client.wsRequest(ctx, fmt.Sprintf("vms/%s/port-forward", name),
		map[string]string{"port": strconv.FormatUint(uint64(port), 10)})
}

func (service *VMsService) StreamEvents(name string) *EventStreamer {
	return NewEventStreamer(service.client, fmt.Sprintf("vms/%s/events", name))
}

func (service *VMsService) Logs(ctx context.Context, name string) (lines []string, err error) {
	var events []v1.Event
	err = service.client.request(ctx, http.MethodGet, fmt.Sprintf("vms/%s/events", name),
		nil, &events, nil)
	if err != nil {
		return
	}
	for _, event := range events {
		if event.Kind == v1.EventKindLogLine {
			lines = append(lines, event.Payload)
		}
	}
	return
}
