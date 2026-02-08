package tests_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/cirruslabs/orchard/internal/execstream"
	"github.com/cirruslabs/orchard/internal/imageconstant"
	"github.com/cirruslabs/orchard/internal/tests/devcontroller"
	"github.com/cirruslabs/orchard/internal/tests/wait"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestVMExec(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	devClient, devController, _ := devcontroller.StartIntegrationTestEnvironment(t)

	vmName := "test-vm-exec-" + uuid.NewString()

	err := devClient.VMs().Create(ctx, &v1.VM{
		Meta: v1.Meta{
			Name: vmName,
		},
		Image:    imageconstant.DefaultMacosImage,
		CPU:      4,
		Memory:   8 * 1024,
		Headless: true,
	})
	require.NoError(t, err)

	require.True(t, wait.Wait(2*time.Minute, func() bool {
		vm, getErr := devClient.VMs().Get(ctx, vmName)
		require.NoError(t, getErr)

		t.Logf("Waiting for the VM to start. Current status: %s", vm.Status)

		return vm.Status == v1.VMStatusRunning || vm.Status == v1.VMStatusFailed
	}), "failed to start a VM")

	vm, err := devClient.VMs().Get(ctx, vmName)
	require.NoError(t, err)
	require.Equal(t, v1.VMStatusRunning, vm.Status)

	execConn, err := dialExec(ctx, devController.Address(), vmName, "sh", []string{
		"-c",
		"echo stdout-line; echo stderr-line >&2; IFS= read -r line; echo stdin:$line; exit 7",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = execConn.Close()
	})

	encoder := execstream.NewEncoder(execConn)
	decoder := execstream.NewDecoder(execConn)

	require.NoError(t, execstream.WriteFrame(encoder, &execstream.Frame{
		Type: execstream.FrameTypeStdin,
		Data: []byte("hello-from-test\\n"),
	}))
	require.NoError(t, execstream.WriteFrame(encoder, &execstream.Frame{
		Type: execstream.FrameTypeStdin,
		Data: []byte{},
	}))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var exitFrame *execstream.Exit

	for {
		var frame execstream.Frame
		require.NoError(t, execstream.ReadFrame(decoder, &frame))

		switch frame.Type {
		case execstream.FrameTypeStdout:
			stdout.Write(frame.Data)
		case execstream.FrameTypeStderr:
			stderr.Write(frame.Data)
		case execstream.FrameTypeExit:
			require.NotNil(t, frame.Exit)
			exitFrame = frame.Exit
		case execstream.FrameTypeError:
			t.Fatalf("unexpected error frame: %s", frame.Error)
		default:
			t.Fatalf("unexpected frame type: %q", frame.Type)
		}

		if exitFrame != nil {
			break
		}
	}

	require.EqualValues(t, 7, exitFrame.Code)
	require.Contains(t, stdout.String(), "stdout-line")
	require.Contains(t, stdout.String(), "stdin:hello-from-test")
	require.Contains(t, stderr.String(), "stderr-line")
}

func dialExec(
	ctx context.Context,
	controllerAddress string,
	vmName string,
	command string,
	args []string,
) (interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
}, error) {
	endpointURL, err := url.Parse(controllerAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to parse controller address: %w", err)
	}

	endpointURL = endpointURL.JoinPath("v1", "vms", vmName, "exec")
	if endpointURL.Scheme == "http" {
		endpointURL.Scheme = "ws"
	} else {
		endpointURL.Scheme = "wss"
	}

	query := endpointURL.Query()
	query.Set("command", command)
	for _, arg := range args {
		query.Add("arg", arg)
	}
	query.Set("wait", "120")
	endpointURL.RawQuery = query.Encode()

	wsConn, resp, err := websocket.Dial(ctx, endpointURL.String(), &websocket.DialOptions{
		HTTPClient: http.DefaultClient,
	})
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}

		return nil, fmt.Errorf("failed to establish exec websocket: %w", err)
	}

	return websocket.NetConn(ctx, wsConn, websocket.MessageText), nil
}
