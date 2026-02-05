package client

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cirruslabs/orchard/pkg/resource/v1"
)

type VMsService struct {
	client *Client
}

type LogsOrder string

const (
	LogsOrderAsc  LogsOrder = "asc"
	LogsOrderDesc LogsOrder = "desc"
)

func ParseLogsOrder(raw string) (LogsOrder, error) {
	order := strings.ToLower(raw)
	switch order {
	case string(LogsOrderAsc):
		return LogsOrderAsc, nil
	case string(LogsOrderDesc):
		return LogsOrderDesc, nil
	default:
		return "", fmt.Errorf("invalid order %q: expected asc or desc", raw)
	}
}

type LogsOptions struct {
	Limit int
	Order LogsOrder
}

type EventsPageOptions struct {
	Limit  int
	Order  LogsOrder
	Cursor string
}

type IssueAccessTokenOptions struct {
	TTLSeconds *uint64
}

func (service *VMsService) Create(ctx context.Context, vm *v1.VM) error {
	err := service.client.request(ctx, http.MethodPost, "vms",
		vm, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (service *VMsService) FindForWorker(ctx context.Context, worker string) ([]v1.VM, error) {
	allVms, err := service.List(ctx, WithListFilters(v1.Filter{
		Path:  "worker",
		Value: worker,
	}))
	if err != nil {
		return nil, err
	}

	var result []v1.VM

	// Backwards compatibility with older Orchard Controllers
	// that do not support the "filter" query parameter
	for _, vmResource := range allVms {
		if vmResource.Worker != worker {
			continue
		}

		result = append(result, vmResource)
	}

	return result, nil
}

func (service *VMsService) List(ctx context.Context, opts ...ListOption) ([]v1.VM, error) {
	params := map[string]string{}

	// Apply options
	for _, opt := range opts {
		opt(params)
	}

	var vms []v1.VM

	err := service.client.request(ctx, http.MethodGet, "vms",
		nil, &vms, params)
	if err != nil {
		return nil, err
	}

	return vms, nil
}

func (service *VMsService) Get(ctx context.Context, name string) (*v1.VM, error) {
	var vm v1.VM

	err := service.client.request(ctx, http.MethodGet, fmt.Sprintf("vms/%s", url.PathEscape(name)),
		nil, &vm, nil)
	if err != nil {
		return nil, err
	}

	return &vm, nil
}

func (service *VMsService) Update(ctx context.Context, vm v1.VM) (*v1.VM, error) {
	var updatedVM v1.VM
	err := service.client.request(ctx, http.MethodPut, fmt.Sprintf("vms/%s", url.PathEscape(vm.Name)),
		vm, &updatedVM, nil)
	if err != nil {
		return &updatedVM, err
	}

	return &updatedVM, nil
}

func (service *VMsService) UpdateState(ctx context.Context, vm v1.VM) (*v1.VM, error) {
	var updatedVM v1.VM
	err := service.client.request(ctx, http.MethodPut, fmt.Sprintf("vms/%s/state", url.PathEscape(vm.Name)),
		vm, &updatedVM, nil)
	if err != nil {
		return &updatedVM, err
	}

	return &updatedVM, nil
}

func (service *VMsService) Delete(ctx context.Context, name string) error {
	err := service.client.request(ctx, http.MethodDelete, fmt.Sprintf("vms/%s", url.PathEscape(name)),
		nil, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (service *VMsService) PortForward(
	ctx context.Context,
	name string,
	port uint16,
	waitSeconds uint16,
) (net.Conn, error) {
	return service.client.wsRequest(ctx, fmt.Sprintf("vms/%s/port-forward", url.PathEscape(name)),
		map[string]string{
			"port": strconv.FormatUint(uint64(port), 10),
			"wait": strconv.FormatUint(uint64(waitSeconds), 10),
		})
}

func (service *VMsService) IP(ctx context.Context, name string, waitSeconds uint16) (string, error) {
	result := struct {
		IP string `json:"ip"`
	}{}

	err := service.client.request(ctx, http.MethodGet, fmt.Sprintf("vms/%s/ip", url.PathEscape(name)),
		nil, &result, map[string]string{
			"wait": strconv.FormatUint(uint64(waitSeconds), 10),
		})
	if err != nil {
		return "", err
	}

	return result.IP, nil
}

func (service *VMsService) IssueAccessToken(
	ctx context.Context,
	name string,
	options IssueAccessTokenOptions,
) (*v1.VMAccessToken, error) {
	request := v1.IssueVMAccessTokenRequest{
		TTLSeconds: options.TTLSeconds,
	}

	var token v1.VMAccessToken

	err := service.client.request(ctx, http.MethodPost, fmt.Sprintf("vms/%s/access-tokens", url.PathEscape(name)),
		request, &token, nil)
	if err != nil {
		return nil, err
	}

	return &token, nil
}

func (service *VMsService) StreamEvents(name string) *EventStreamer {
	return NewEventStreamer(service.client, fmt.Sprintf("vms/%s/events", url.PathEscape(name)))
}

func (service *VMsService) Logs(ctx context.Context, name string) (lines []string, err error) {
	return service.LogsWithOptions(ctx, name, LogsOptions{})
}

func (service *VMsService) LogsWithOptions(ctx context.Context, name string, options LogsOptions) (lines []string, err error) {
	var events []v1.Event
	params := map[string]string{}
	if options.Limit > 0 {
		params["limit"] = strconv.Itoa(options.Limit)
	}
	if options.Order != "" {
		params["order"] = string(options.Order)
	}
	if len(params) == 0 {
		params = nil
	}
	err = service.client.request(ctx, http.MethodGet, fmt.Sprintf("vms/%s/events", url.PathEscape(name)),
		nil, &events, params)
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

func (service *VMsService) EventsPage(
	ctx context.Context,
	name string,
	options EventsPageOptions,
) (events []v1.Event, nextCursor string, err error) {
	params := map[string]string{}
	if options.Limit > 0 {
		params["limit"] = strconv.Itoa(options.Limit)
	}
	if options.Order != "" {
		params["order"] = string(options.Order)
	}
	if options.Cursor != "" {
		params["cursor"] = options.Cursor
	}
	if len(params) == 0 {
		params = nil
	}

	headers, err := service.client.requestWithHeaders(ctx, http.MethodGet, fmt.Sprintf("vms/%s/events", url.PathEscape(name)),
		nil, &events, params)
	if err != nil {
		return nil, "", err
	}

	return events, headers.Get("X-Next-Cursor"), nil
}
