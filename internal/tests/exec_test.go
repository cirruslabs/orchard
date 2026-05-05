package tests_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cirruslabs/orchard/internal/execstream"
	"github.com/cirruslabs/orchard/internal/tests/devcontroller"
	"github.com/cirruslabs/orchard/internal/tests/platformdependent"
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

func TestVMExecWithOptions(t *testing.T) {
	devClient, vmName := prepareForExec(t)

	script := "sh -c 'printf \"%s|%s|%s\" \"$GREETING\" \"$QUOTE\" \"$PWD\"'"

	wsConn, err := devClient.VMs().ExecSession(t.Context(), vmName, client.ExecSessionOptions{
		Command: script,
		Env: map[string]string{
			"GREETING": "Hello, World!",
			"QUOTE":    "O'Reilly",
		},
		Workdir:     "/tmp",
		WaitSeconds: 30,
	})
	require.NoError(t, err)
	defer wsConn.CloseNow()

	var stdout bytes.Buffer
	var exitFrame *execstream.Frame

	for exitFrame == nil {
		frame := readFrame(t, wsConn)

		switch frame.Type {
		case execstream.FrameTypeStdout:
			stdout.Write(frame.Data)
		case execstream.FrameTypeExit:
			exitFrame = frame
		default:
			t.Fatalf("unexpected frame type %q", frame.Type)
		}
	}

	require.EqualValues(t, 0, exitFrame.Exit.Code)
	require.Equal(t, "Hello, World!|O'Reilly|/tmp", stdout.String())
}

func TestVMExecTTYResize(t *testing.T) {
	devClient, vmName := prepareForExec(t)

	wsConn, err := devClient.VMs().ExecSession(t.Context(), vmName, client.ExecSessionOptions{
		Command:     "sh -c 'stty size; read -r line; stty size'",
		Interactive: true,
		TTY:         true,
		Rows:        24,
		Cols:        80,
		WaitSeconds: 30,
	})
	require.NoError(t, err)
	defer wsConn.CloseNow()

	firstFrame := readFrame(t, wsConn)
	require.Equal(t, execstream.FrameTypeStdout, firstFrame.Type)
	require.Contains(t, string(firstFrame.Data), "24 80")

	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{
		Type: execstream.FrameTypeResize,
		Terminal: &execstream.TerminalSize{
			Rows: 40,
			Cols: 120,
		},
	})
	require.NoError(t, err)
	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{
		Type: execstream.FrameTypeStdin,
		Data: []byte("continue\n"),
	})
	require.NoError(t, err)

	var stdout bytes.Buffer
	var exitFrame *execstream.Frame
	for exitFrame == nil {
		frame := readFrame(t, wsConn)

		switch frame.Type {
		case execstream.FrameTypeStdout:
			stdout.Write(frame.Data)
		case execstream.FrameTypeExit:
			exitFrame = frame
		default:
			t.Fatalf("unexpected frame type %q", frame.Type)
		}
	}

	require.Contains(t, stdout.String(), "40 120")
	require.EqualValues(t, 0, exitFrame.Exit.Code)
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

func TestVMExecSessionReconnectHistory(t *testing.T) {
	devClient, vmName := prepareForExec(t)
	sessionID := uuid.NewString()

	wsConn, err := devClient.VMs().ExecSession(t.Context(), vmName, client.ExecSessionOptions{
		Command:     "sh -c 'echo first; sleep 1; echo second'",
		WaitSeconds: 30,
		Session:     sessionID,
	})
	require.NoError(t, err)

	firstFrame := readFrame(t, wsConn)
	require.Equal(t, execstream.FrameTypeStdout, firstFrame.Type)
	require.Equal(t, "first\n", string(firstFrame.Data))
	require.EqualValues(t, 1, firstFrame.Watermark)

	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{Type: execstream.FrameTypeDetach})
	require.NoError(t, err)
	_ = wsConn.CloseNow()

	// Let the detached process finish so this test verifies partial replay
	// without relying on live-output timing.
	time.Sleep(2 * time.Second)

	wsConn, err = devClient.VMs().ExecSession(t.Context(), vmName, client.ExecSessionOptions{
		WaitSeconds: 30,
		Session:     sessionID,
	})
	require.NoError(t, err)
	defer wsConn.CloseNow()

	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{
		Type:      execstream.FrameTypeHistory,
		Watermark: firstFrame.Watermark,
	})
	require.NoError(t, err)

	frames := readFramesUntilExit(t, wsConn)
	require.Len(t, framesByType(frames, execstream.FrameTypeStdout), 1)
	require.Equal(t, "second\n", string(framesByType(frames, execstream.FrameTypeStdout)[0].Data))
	require.EqualValues(t, 0, framesByType(frames, execstream.FrameTypeExit)[0].Exit.Code)
}

func TestVMExecSessionReconnectAfterExit(t *testing.T) {
	devClient, vmName := prepareForExec(t)
	sessionID := uuid.NewString()

	wsConn, err := devClient.VMs().ExecSession(t.Context(), vmName, client.ExecSessionOptions{
		Command:     "sh -c 'echo replay-me'",
		WaitSeconds: 30,
		Session:     sessionID,
	})
	require.NoError(t, err)

	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{Type: execstream.FrameTypeDetach})
	require.NoError(t, err)
	_ = wsConn.CloseNow()

	time.Sleep(time.Second)

	wsConn, err = devClient.VMs().ExecSession(t.Context(), vmName, client.ExecSessionOptions{
		WaitSeconds: 30,
		Session:     sessionID,
	})
	require.NoError(t, err)
	defer wsConn.CloseNow()

	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{Type: execstream.FrameTypeHistory})
	require.NoError(t, err)

	frames := readFramesUntilExit(t, wsConn)
	require.Equal(t, "replay-me\n", string(framesByType(frames, execstream.FrameTypeStdout)[0].Data))
	require.EqualValues(t, 0, framesByType(frames, execstream.FrameTypeExit)[0].Exit.Code)
}

func TestVMExecSessionReplayPreservesStreams(t *testing.T) {
	devClient, vmName := prepareForExec(t)
	sessionID := uuid.NewString()

	wsConn, err := devClient.VMs().ExecSession(t.Context(), vmName, client.ExecSessionOptions{
		Command:     "sh -c 'echo out1; sleep 1; echo err1 >&2; sleep 1; echo out2; sleep 1; echo err2 >&2'",
		WaitSeconds: 30,
		Session:     sessionID,
	})
	require.NoError(t, err)

	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{Type: execstream.FrameTypeDetach})
	require.NoError(t, err)
	_ = wsConn.CloseNow()

	time.Sleep(4 * time.Second)

	wsConn, err = devClient.VMs().ExecSession(t.Context(), vmName, client.ExecSessionOptions{
		WaitSeconds: 30,
		Session:     sessionID,
	})
	require.NoError(t, err)
	defer wsConn.CloseNow()

	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{Type: execstream.FrameTypeHistory})
	require.NoError(t, err)

	frames := readFramesUntilExit(t, wsConn)
	require.Equal(t, []execstream.FrameType{
		execstream.FrameTypeStdout,
		execstream.FrameTypeStderr,
		execstream.FrameTypeStdout,
		execstream.FrameTypeStderr,
		execstream.FrameTypeExit,
	}, frameTypes(frames))
	require.Equal(t, "out1\n", string(frames[0].Data))
	require.Equal(t, "err1\n", string(frames[1].Data))
	require.Equal(t, "out2\n", string(frames[2].Data))
	require.Equal(t, "err2\n", string(frames[3].Data))
}

func TestVMExecSessionStdinSurvivesReconnect(t *testing.T) {
	devClient, vmName := prepareForExec(t)
	sessionID := uuid.NewString()

	wsConn, err := devClient.VMs().ExecSession(t.Context(), vmName, client.ExecSessionOptions{
		Command:     "/bin/cat",
		Interactive: true,
		WaitSeconds: 30,
		Session:     sessionID,
	})
	require.NoError(t, err)

	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{
		Type: execstream.FrameTypeStdin,
		Data: []byte("one\n"),
	})
	require.NoError(t, err)

	frame := readFrame(t, wsConn)
	require.Equal(t, execstream.FrameTypeStdout, frame.Type)
	require.Equal(t, "one\n", string(frame.Data))

	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{Type: execstream.FrameTypeDetach})
	require.NoError(t, err)
	_ = wsConn.CloseNow()

	wsConn, err = devClient.VMs().ExecSession(t.Context(), vmName, client.ExecSessionOptions{
		WaitSeconds: 30,
		Session:     sessionID,
	})
	require.NoError(t, err)
	defer wsConn.CloseNow()

	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{
		Type: execstream.FrameTypeStdin,
		Data: []byte("two\n"),
	})
	require.NoError(t, err)
	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{
		Type: execstream.FrameTypeStdin,
		Data: []byte{},
	})
	require.NoError(t, err)
	err = execstream.WriteFrame(t.Context(), wsConn, &execstream.Frame{Type: execstream.FrameTypeHistory})
	require.NoError(t, err)

	frames := readFramesUntilExit(t, wsConn)
	stdoutFrames := framesByType(frames, execstream.FrameTypeStdout)
	require.Len(t, stdoutFrames, 2)
	require.Equal(t, "one\n", string(stdoutFrames[0].Data))
	require.Equal(t, "two\n", string(stdoutFrames[1].Data))
	require.EqualValues(t, 0, framesByType(frames, execstream.FrameTypeExit)[0].Exit.Code)
}

func prepareForExec(t *testing.T) (*client.Client, string) {
	devClient, _, _ := devcontroller.StartIntegrationTestEnvironment(t)

	vmName := "test-vm-exec-" + uuid.NewString()

	err := devClient.VMs().Create(t.Context(), platformdependent.VM(vmName))
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
	t.Helper()

	var frame execstream.Frame

	readCtx, readCtxCancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer readCtxCancel()

	messageType, payloadBytes, err := wsConn.Read(readCtx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageText, messageType)

	err = json.Unmarshal(payloadBytes, &frame)
	require.NoError(t, err)
	if frame.Type == execstream.FrameTypeError {
		require.FailNowf(t, "exec stream error", "%s", frame.Error)
	}

	return &frame
}

func readFramesUntilExit(t *testing.T, wsConn *websocket.Conn) []*execstream.Frame {
	t.Helper()

	var frames []*execstream.Frame

	for {
		frame := readFrame(t, wsConn)
		if frame.Type == execstream.FrameTypeNoMoreHistory {
			continue
		}

		frames = append(frames, frame)
		if frame.Type == execstream.FrameTypeExit {
			return frames
		}
	}
}

func framesByType(frames []*execstream.Frame, frameType execstream.FrameType) []*execstream.Frame {
	var result []*execstream.Frame

	for _, frame := range frames {
		if frame.Type == frameType {
			result = append(result, frame)
		}
	}

	return result
}

func frameTypes(frames []*execstream.Frame) []execstream.FrameType {
	var result []execstream.FrameType

	for _, frame := range frames {
		result = append(result, frame.Type)
	}

	return result
}
