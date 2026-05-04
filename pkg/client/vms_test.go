package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestExecSessionBuildsReconnectableQuery(t *testing.T) {
	var query map[string][]string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query = request.URL.Query()

		conn, err := websocket.Accept(writer, request, nil)
		require.NoError(t, err)
		defer conn.CloseNow()
	}))
	defer server.Close()

	devClient, err := New(WithAddress(server.URL))
	require.NoError(t, err)

	conn, err := devClient.VMs().ExecSession(t.Context(), "vm", ExecSessionOptions{
		Command:     "echo hello",
		Stdin:       true,
		WaitSeconds: 7,
		Session:     "resume-me",
	})
	require.NoError(t, err)
	defer conn.CloseNow()

	require.Equal(t, []string{"echo hello"}, query["command"])
	require.Equal(t, []string{"true"}, query["stdin"])
	require.Equal(t, []string{"7"}, query["wait"])
	require.Equal(t, []string{"resume-me"}, query["session"])
}
