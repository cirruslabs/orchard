package tests_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/cirruslabs/orchard/internal/imageconstant"
	"github.com/cirruslabs/orchard/internal/tests/devcontroller"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/require"
)

func TestListVMEventsPagination(t *testing.T) {
	devClient, devController, _ := devcontroller.StartIntegrationTestEnvironmentWithAdditionalOpts(t,
		false, nil,
		true, nil,
	)

	ctx := context.Background()
	vm := v1.VM{
		Meta:     v1.Meta{Name: "test-vm"},
		Image:    imageconstant.DefaultMacosImage,
		CPU:      1,
		Memory:   1024,
		Headless: true,
	}
	require.NoError(t, devClient.VMs().Create(ctx, &vm))

	events := []v1.Event{
		{Kind: v1.EventKindLogLine, Timestamp: 1, Payload: "one"},
		{Kind: v1.EventKindLogLine, Timestamp: 2, Payload: "two"},
		{Kind: v1.EventKindLogLine, Timestamp: 3, Payload: "three"},
		{Kind: v1.EventKindLogLine, Timestamp: 4, Payload: "four"},
	}
	appendVMEvents(t, devController.Address(), vm.Name, events)

	page, cursor := fetchVMEventsPage(t, ctx, devClient, vm.Name, client.EventsPageOptions{Limit: 2})
	require.Equal(t, events[:2], page)
	require.NotEmpty(t, cursor)

	page2, cursor2 := fetchVMEventsPage(t, ctx, devClient, vm.Name, client.EventsPageOptions{
		Limit:  2,
		Cursor: cursor,
	})
	require.Equal(t, events[2:], page2)
	require.Empty(t, cursor2)

	descPage, descCursor := fetchVMEventsPage(t, ctx, devClient, vm.Name, client.EventsPageOptions{
		Limit: 2,
		Order: client.LogsOrderDesc,
	})
	require.Equal(t, []v1.Event{events[3], events[2]}, descPage)
	require.NotEmpty(t, descCursor)

	descPage2, descCursor2 := fetchVMEventsPage(t, ctx, devClient, vm.Name, client.EventsPageOptions{
		Limit:  2,
		Order:  client.LogsOrderDesc,
		Cursor: descCursor,
	})
	require.Equal(t, []v1.Event{events[1], events[0]}, descPage2)
	require.Empty(t, descCursor2)

	lines, err := devClient.VMs().LogsWithOptions(ctx, vm.Name, client.LogsOptions{
		Limit: 2,
		Order: client.LogsOrderDesc,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"four", "three"}, lines)
}

func appendVMEvents(t *testing.T, baseURL, name string, events []v1.Event) {
	t.Helper()

	endpoint, err := url.JoinPath(baseURL, "v1", "vms", name, "events")
	require.NoError(t, err)

	body, err := json.Marshal(events)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func fetchVMEventsPage(
	t *testing.T,
	ctx context.Context,
	devClient *client.Client,
	name string,
	options client.EventsPageOptions,
) ([]v1.Event, string) {
	t.Helper()

	events, cursor, err := devClient.VMs().EventsPage(ctx, name, options)
	require.NoError(t, err)

	return events, cursor
}
