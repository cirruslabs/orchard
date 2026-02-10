package tests_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/cirruslabs/orchard/internal/execstream"
	"github.com/cirruslabs/orchard/internal/imageconstant"
	"github.com/cirruslabs/orchard/internal/tests/devcontroller"
	"github.com/cirruslabs/orchard/internal/tests/wait"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestVMExecWithoutStdin(t *testing.T) {
	devClient, vmName := prepareForExec(t)

	// Run a command
	wsConn, err := devClient.VMs().Exec(t.Context(), vmName, "/bin/echo -n 'Hello, World!'",
		false, 30)
	require.NoError(t, err)
	defer wsConn.CloseNow()

	// Ensure that the command outputs "Hello, World!" and terminates successfully
	frame := readFrame(t, wsConn)
	require.Equal(t, execstream.FrameTypeStdout, frame.Type)
	require.Equal(t, "Hello, World!", string(frame.Data))

	frame = readFrame(t, wsConn)
	require.Equal(t, execstream.FrameTypeExit, frame.Type)
	require.EqualValues(t, 0, frame.Exit.Code)

	// Ensure that Orchard Controller gracefully terminates the WebSocket connection
	_, _, err = wsConn.Read(t.Context())
	var closeError websocket.CloseError
	require.ErrorAs(t, err, &closeError)
	require.Equal(t, websocket.StatusNormalClosure, closeError.Code)
}

func TestVMExecWithStdin(t *testing.T) {
	devClient, vmName := prepareForExec(t)

	// Run a command
	wsConn, err := devClient.VMs().Exec(t.Context(), vmName, "/bin/cat", true, 30)
	require.NoError(t, err)
	defer wsConn.CloseNow()

	// Populate and close the command's standard input
	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{
		Type: execstream.FrameTypeStdin,
		Data: []byte("Hello, World!\n"),
	})
	require.NoError(t, err)

	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{
		Type: execstream.FrameTypeStdin,
		Data: []byte{},
	})
	require.NoError(t, err)

	// Ensure that the command outputs "Hello, World!\n" and terminates successfully
	frame := readFrame(t, wsConn)
	require.Equal(t, execstream.FrameTypeStdout, frame.Type)
	require.Equal(t, "Hello, World!\n", string(frame.Data))

	frame = readFrame(t, wsConn)
	require.Equal(t, execstream.FrameTypeExit, frame.Type)
	require.EqualValues(t, 0, frame.Exit.Code)

	// Ensure that Orchard Controller gracefully terminates the WebSocket connection
	_, _, err = wsConn.Read(t.Context())
	var closeError websocket.CloseError
	require.ErrorAs(t, err, &closeError)
	require.Equal(t, websocket.StatusNormalClosure, closeError.Code)
}

func TestVMExecScript(t *testing.T) {
	devClient, vmName := prepareForExec(t)

	script := "sh -c 'echo stdout-line; echo stderr-line >&2; IFS= read -r line; echo stdin:$line; exit 7'"

	wsConn, err := devClient.VMs().Exec(t.Context(), vmName, script, true, 30)
	require.NoError(t, err)
	defer wsConn.CloseNow()

	// Populate and close the command's standard input
	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{
		Type: execstream.FrameTypeStdin,
		Data: []byte("hello-from-test\n"),
	})
	require.NoError(t, err)

	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{
		Type: execstream.FrameTypeStdin,
		Data: []byte{},
	})
	require.NoError(t, err)

	// Collect output and wait for command's exit
	var stdout, stderr bytes.Buffer
	var exitFrame *execstream.Frame

	for exitFrame == nil {
		frame := readFrame(t, wsConn)

		switch frame.Type {
		case execstream.FrameTypeStdout:
			stdout.Write(frame.Data)
		case execstream.FrameTypeStderr:
			stderr.Write(frame.Data)
		case execstream.FrameTypeExit:
			exitFrame = frame
		default:
			t.Fatalf("unexpected frame type %q", frame.Type)
		}
	}

	// Ensure that we've observed everything as per script
	require.EqualValues(t, 7, exitFrame.Exit.Code)
	require.Equal(t, "stdout-line\nstdin:hello-from-test\n", stdout.String())
	require.Equal(t, "stderr-line\n", stderr.String())

	// Ensure that Orchard Controller gracefully terminates the WebSocket connection
	_, _, err = wsConn.Read(t.Context())
	var closeError websocket.CloseError
	require.ErrorAs(t, err, &closeError)
	require.Equal(t, websocket.StatusNormalClosure, closeError.Code)
}

func prepareForExec(t *testing.T) (*client.Client, string) {
	devClient, _, _ := devcontroller.StartIntegrationTestEnvironment(t)

	vmName := "test-vm-exec-" + uuid.NewString()

	err := devClient.VMs().Create(t.Context(), &v1.VM{
		Meta: v1.Meta{
			Name: vmName,
		},
		Image:    imageconstant.DefaultMacosImage,
		Headless: true,
	})
	require.NoError(t, err)

	require.True(t, wait.Wait(2*time.Minute, func() bool {
		vm, err := devClient.VMs().Get(t.Context(), vmName)
		require.NoError(t, err)

		t.Logf("Waiting for the VM to start. Current status: %s", vm.Status)

		return vm.Status == v1.VMStatusRunning
	}), "failed to start a VM")

	return devClient, vmName
}

func readFrame(t *testing.T, wsConn *websocket.Conn) *execstream.Frame {
	var frame execstream.Frame

	messageType, payloadBytes, err := wsConn.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, websocket.MessageText, messageType)

	err = json.Unmarshal(payloadBytes, &frame)
	require.NoError(t, err)

	return &frame
}
