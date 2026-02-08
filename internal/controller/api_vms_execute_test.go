//nolint:testpackage // we need to have access to unexported helpers
package controller

import (
	"bytes"
	"io"
	"testing"

	"github.com/cirruslabs/orchard/internal/execstream"
	"github.com/stretchr/testify/require"
)

type recordingWriteCloser struct {
	bytes.Buffer
	closed bool
}

func (writer *recordingWriteCloser) Close() error {
	writer.closed = true

	return nil
}

func TestStreamExecuteClientFramesWritesInputAndClosesOnEOFFrame(t *testing.T) {
	var input bytes.Buffer
	encoder := execstream.NewEncoder(&input)

	require.NoError(t, execstream.WriteFrame(encoder, &execstream.Frame{
		Type: execstream.FrameTypeStdin,
		Data: []byte("hello"),
	}))
	require.NoError(t, execstream.WriteFrame(encoder, &execstream.Frame{
		Type: execstream.FrameTypeResize,
	}))
	require.NoError(t, execstream.WriteFrame(encoder, &execstream.Frame{
		Type: execstream.FrameTypeStdin,
		Data: []byte{},
	}))

	decoder := execstream.NewDecoder(&input)
	stdin := &recordingWriteCloser{}
	errCh := make(chan error, 1)

	streamExecuteClientFrames(decoder, stdin, errCh)

	require.NoError(t, <-errCh)
	require.True(t, stdin.closed)
	require.Equal(t, "hello", stdin.String())
}

func TestStreamExecuteClientFramesUnsupportedType(t *testing.T) {
	var input bytes.Buffer
	encoder := execstream.NewEncoder(&input)

	require.NoError(t, execstream.WriteFrame(encoder, &execstream.Frame{
		Type: execstream.FrameTypeStdout,
		Data: []byte("output"),
	}))

	decoder := execstream.NewDecoder(&input)
	stdin := &recordingWriteCloser{}
	errCh := make(chan error, 1)

	streamExecuteClientFrames(decoder, stdin, errCh)

	require.EqualError(t, <-errCh, "unsupported frame type \"stdout\" received from client")
	require.False(t, stdin.closed)
}

func TestStreamExecuteClientFramesClosesStdinOnDecodeError(t *testing.T) {
	decoder := execstream.NewDecoder(bytes.NewBuffer(nil))
	stdin := &recordingWriteCloser{}
	errCh := make(chan error, 1)

	streamExecuteClientFrames(decoder, stdin, errCh)

	require.ErrorIs(t, <-errCh, io.EOF)
	require.True(t, stdin.closed)
}

func TestStreamExecuteOutputEmitsFrameAndSignalsDone(t *testing.T) {
	outputCh := make(chan execstream.Frame, 1)
	doneCh := make(chan struct{}, 1)
	errCh := make(chan error, 1)

	streamExecuteOutput(bytes.NewBufferString("payload"),
		execstream.FrameTypeStderr, outputCh, doneCh, errCh)

	select {
	case frame := <-outputCh:
		require.Equal(t, execstream.FrameTypeStderr, frame.Type)
		require.Equal(t, []byte("payload"), frame.Data)
	default:
		t.Fatal("expected frame")
	}

	select {
	case <-doneCh:
	default:
		t.Fatal("expected done signal")
	}

	select {
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	default:
	}
}

func TestBuildSSHCommandQuotesArguments(t *testing.T) {
	result := buildSSHCommand("echo", []string{"hello world", "a'b", ""})

	require.Equal(t, "'echo' 'hello world' 'a'\\''b' ''", result)
}
