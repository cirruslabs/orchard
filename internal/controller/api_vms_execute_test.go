//nolint:testpackage // we need to have access to unexported helpers
package controller

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

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

func TestConsumeClientInputFramesWritesInputAndClosesOnEOFFrame(t *testing.T) {
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

	consumeClientInputFrames(decoder, stdin, errCh)

	require.NoError(t, <-errCh)
	require.True(t, stdin.closed)
	require.Equal(t, "hello", stdin.String())
}

func TestConsumeClientInputFramesUnsupportedType(t *testing.T) {
	var input bytes.Buffer
	encoder := execstream.NewEncoder(&input)

	require.NoError(t, execstream.WriteFrame(encoder, &execstream.Frame{
		Type: execstream.FrameTypeStdout,
		Data: []byte("output"),
	}))

	decoder := execstream.NewDecoder(&input)
	stdin := &recordingWriteCloser{}
	errCh := make(chan error, 1)

	consumeClientInputFrames(decoder, stdin, errCh)

	require.EqualError(t, <-errCh, "unsupported frame type \"stdout\" received from client")
	require.False(t, stdin.closed)
}

func TestConsumeClientInputFramesClosesStdinOnDecodeError(t *testing.T) {
	decoder := execstream.NewDecoder(bytes.NewBuffer(nil))
	stdin := &recordingWriteCloser{}
	errCh := make(chan error, 1)

	consumeClientInputFrames(decoder, stdin, errCh)

	require.ErrorIs(t, <-errCh, io.EOF)
	require.True(t, stdin.closed)
}

func TestForwardSSHOutputFramesEmitsFrameAndSignalsDone(t *testing.T) {
	outputFrameCh := make(chan execstream.Frame, 1)
	outputDoneCh := make(chan struct{}, 1)
	outputErrCh := make(chan error, 1)

	forwardSSHOutputFrames(context.Background(), bytes.NewBufferString("payload"),
		execstream.FrameTypeStderr, outputFrameCh, outputDoneCh, outputErrCh)

	select {
	case frame := <-outputFrameCh:
		require.Equal(t, execstream.FrameTypeStderr, frame.Type)
		require.Equal(t, []byte("payload"), frame.Data)
	default:
		t.Fatal("expected frame")
	}

	select {
	case <-outputDoneCh:
	default:
		t.Fatal("expected done signal")
	}

	select {
	case err := <-outputErrCh:
		t.Fatalf("unexpected error: %v", err)
	default:
	}
}

func TestForwardSSHOutputFramesStopsWhenContextCancelledWhileOutputChannelIsBlocked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outputFrameCh := make(chan execstream.Frame, 1)
	outputFrameCh <- execstream.Frame{
		Type: execstream.FrameTypeStdout,
		Data: []byte("occupied"),
	}

	outputDoneCh := make(chan struct{}, 1)
	outputErrCh := make(chan error, 1)

	finished := make(chan struct{})
	go func() {
		forwardSSHOutputFrames(ctx, bytes.NewBufferString("payload"),
			execstream.FrameTypeStdout, outputFrameCh, outputDoneCh, outputErrCh)
		close(finished)
	}()

	select {
	case <-finished:
		t.Fatal("forwardSSHOutputFrames unexpectedly returned before context cancellation")
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("forwardSSHOutputFrames did not return after context cancellation")
	}

	select {
	case <-outputDoneCh:
	default:
		t.Fatal("expected done signal")
	}
}

func TestFirstExecuteOutputErrorReturnsFirstNonNilError(t *testing.T) {
	outputErrCh := make(chan error, 3)
	outputErrCh <- nil
	outputErrCh <- errors.New("first error")
	outputErrCh <- errors.New("second error")

	require.EqualError(t, firstExecuteOutputError(outputErrCh), "first error")
}

func TestBuildSSHCommandQuotesArguments(t *testing.T) {
	result := buildSSHCommand("echo", []string{"hello world", "a'b", ""})

	require.Equal(t, "'echo' 'hello world' 'a'\\''b' ''", result)
}
