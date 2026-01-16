//nolint:testpackage // we need access to Controller for this test
package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/controller/store/badger"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestListVMEventsPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logger := zap.NewNop().Sugar()
	store, err := badger.NewBadgerStore(t.TempDir(), true, logger)
	require.NoError(t, err)

	controller := &Controller{
		store:                store,
		logger:               logger,
		insecureAuthDisabled: true,
	}

	vm := v1.VM{
		Meta: v1.Meta{Name: "test-vm"},
		UID:  "vm-uid",
	}

	err = store.Update(func(txn storepkg.Transaction) error {
		return txn.SetVM(vm)
	})
	require.NoError(t, err)

	events := []v1.Event{
		{Kind: v1.EventKindLogLine, Timestamp: 1, Payload: "one"},
		{Kind: v1.EventKindLogLine, Timestamp: 2, Payload: "two"},
		{Kind: v1.EventKindLogLine, Timestamp: 3, Payload: "three"},
		{Kind: v1.EventKindLogLine, Timestamp: 4, Payload: "four"},
	}
	err = store.Update(func(txn storepkg.Transaction) error {
		return txn.AppendEvents(events, "vms", vm.UID)
	})
	require.NoError(t, err)

	page, cursor := fetchVMEventsPage(t, controller, vm.Name, "limit=2")
	require.Equal(t, events[:2], page)
	require.NotEmpty(t, cursor)

	page2, cursor2 := fetchVMEventsPage(t, controller, vm.Name, "limit=2&cursor="+url.QueryEscape(cursor))
	require.Equal(t, events[2:], page2)
	require.Empty(t, cursor2)
}

func fetchVMEventsPage(
	t *testing.T,
	controller *Controller,
	name string,
	query string,
) ([]v1.Event, string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	path := "/vms/" + name + "/events"
	if query != "" {
		path += "?" + query
	}
	ctx.Request = httptest.NewRequest(http.MethodGet, path, nil)
	ctx.Params = gin.Params{{Key: "name", Value: name}}

	controller.listVMEvents(ctx).Respond(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var events []v1.Event
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &events))

	return events, recorder.Header().Get("X-Next-Cursor")
}
