package controller

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cirruslabs/orchard/internal/execstream"
	"github.com/stretchr/testify/require"
)

type fakeExec struct {
	stdin      io.WriteCloser
	run        func(context.Context, string, chan<- *execstream.Frame) error
	closeCalls atomic.Int32
}

func (exec *fakeExec) Stdin() io.WriteCloser {
	return exec.stdin
}

func (exec *fakeExec) Run(
	ctx context.Context,
	command string,
	outgoingFrames chan<- *execstream.Frame,
) error {
	if exec.run != nil {
		return exec.run(ctx, command, outgoingFrames)
	}

	return nil
}

func (exec *fakeExec) Close() error {
	exec.closeCalls.Add(1)

	return nil
}

func newManualExecSessionForTest(
	key execSessionKey,
	registry *execSessionRegistry,
) *execSession {
	ctx, cancel := context.WithCancel(context.Background())

	return &execSession{
		key:         key,
		command:     "echo test",
		exec:        &fakeExec{},
		registry:    registry,
		exitTTL:     time.Minute,
		policy:      reconnectableExecSessionPolicy,
		ctx:         ctx,
		cancel:      cancel,
		subscribers: map[*execSessionSubscriber]struct{}{},
		done:        make(chan struct{}),
	}
}

func TestExecSessionRegistryGetOrCreateReusesInflightCreation(t *testing.T) {
	registry := newExecSessionRegistry()
	key := execSessionKey{vmName: "vm", sessionID: "session"}

	createStarted := make(chan struct{})
	releaseCreate := make(chan struct{})
	var createCalls atomic.Int32

	create := func() (*execSession, error) {
		createCalls.Add(1)
		close(createStarted)
		<-releaseCreate

		return newManualExecSessionForTest(key, registry), nil
	}

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_, _, err := registry.getOrCreate(context.Background(), key, create)
		require.NoError(t, err)
	}()

	<-createStarted

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		_, created, err := registry.getOrCreate(context.Background(), key, create)
		require.NoError(t, err)
		require.False(t, created)
	}()

	close(releaseCreate)

	<-firstDone
	<-secondDone
	require.EqualValues(t, 1, createCalls.Load())
}

func TestExecSessionStartRunsCommandOnlyOnce(t *testing.T) {
	var runCalls atomic.Int32
	runStarted := make(chan struct{})

	session := newExecSession(
		execSessionKey{vmName: "vm", sessionID: "session"},
		"echo test",
		&fakeExec{
			run: func(ctx context.Context, _ string, _ chan<- *execstream.Frame) error {
				runCalls.Add(1)
				close(runStarted)
				<-ctx.Done()

				return ctx.Err()
			},
		},
		nil,
		nil,
		time.Minute,
		reconnectableExecSessionPolicy,
	)
	defer session.close()

	session.start()
	session.start()

	<-runStarted
	require.EqualValues(t, 1, runCalls.Load())
}

func TestExecSessionHistoryReplayAndAck(t *testing.T) {
	registry := newExecSessionRegistry()
	session := newManualExecSessionForTest(execSessionKey{vmName: "vm", sessionID: "session"}, registry)

	session.recordFrame(&execstream.Frame{Type: execstream.FrameTypeStdout, Data: []byte("out")})
	session.recordFrame(&execstream.Frame{Type: execstream.FrameTypeStderr, Data: []byte("err")})
	session.recordFrame(&execstream.Frame{
		Type: execstream.FrameTypeExit,
		Exit: &execstream.Exit{Code: 7},
	})

	subscriber, err := session.attach()
	require.NoError(t, err)

	session.sendHistory(subscriber, 0)

	require.Equal(t, execstream.FrameTypeStdout, (<-subscriber.frames).Type)
	require.Equal(t, execstream.FrameTypeStderr, (<-subscriber.frames).Type)
	require.Equal(t, execstream.FrameTypeExit, (<-subscriber.frames).Type)
	noMoreHistory := <-subscriber.frames
	require.Equal(t, execstream.FrameTypeNoMoreHistory, noMoreHistory.Type)
	require.EqualValues(t, 3, noMoreHistory.Watermark)

	session.ack(2)
	require.Len(t, session.frames, 1)
	require.EqualValues(t, 3, session.frames[0].frame.Watermark)
}

func TestExecSessionDetachKeepsProcessAlive(t *testing.T) {
	registry := newExecSessionRegistry()
	session := newManualExecSessionForTest(execSessionKey{vmName: "vm", sessionID: "session"}, registry)
	exec := session.exec.(*fakeExec)

	subscriber, err := session.attach()
	require.NoError(t, err)

	session.detach(subscriber)

	require.False(t, session.closed)
	require.EqualValues(t, 0, exec.closeCalls.Load())
}

func TestLegacyExecSessionDetachStopsProcess(t *testing.T) {
	registry := newExecSessionRegistry()
	session := newManualExecSessionForTest(execSessionKey{vmName: "vm", sessionID: "session"}, registry)
	session.policy = legacyExecSessionPolicy
	exec := session.exec.(*fakeExec)

	subscriber, err := session.attach()
	require.NoError(t, err)

	session.detach(subscriber)

	require.True(t, session.closed)
	require.EqualValues(t, 1, exec.closeCalls.Load())
}

func TestLegacyExecSessionDoesNotRetainReplayHistory(t *testing.T) {
	registry := newExecSessionRegistry()
	session := newManualExecSessionForTest(execSessionKey{vmName: "vm", sessionID: "session"}, registry)
	session.policy = legacyExecSessionPolicy

	session.recordFrame(&execstream.Frame{Type: execstream.FrameTypeStdout, Data: []byte("out")})

	require.Empty(t, session.frames)
	require.Zero(t, session.nextWatermark)
}

func TestExecSessionCloseIfUnusedClosesIdleSession(t *testing.T) {
	registry := newExecSessionRegistry()
	key := execSessionKey{vmName: "vm", sessionID: "session"}
	session := newManualExecSessionForTest(key, registry)
	exec := session.exec.(*fakeExec)
	registry.sessions[key] = session

	session.closeIfUnused()

	require.True(t, session.closed)
	require.EqualValues(t, 1, exec.closeCalls.Load())
}

func TestExecSessionCloseIfUnusedKeepsAttachedSession(t *testing.T) {
	registry := newExecSessionRegistry()
	session := newManualExecSessionForTest(execSessionKey{vmName: "vm", sessionID: "session"}, registry)

	_, err := session.attach()
	require.NoError(t, err)

	session.closeIfUnused()

	require.False(t, session.closed)
}

func TestExecSessionCloseStopsProcessAndRemovesRegistryEntry(t *testing.T) {
	registry := newExecSessionRegistry()
	key := execSessionKey{vmName: "vm", sessionID: "session"}
	session := newManualExecSessionForTest(key, registry)
	exec := session.exec.(*fakeExec)
	registry.sessions[key] = session

	session.close()

	require.True(t, session.closed)
	require.EqualValues(t, 1, exec.closeCalls.Load())
	_, ok := registry.get(key)
	require.False(t, ok)
}

func TestExecSessionFinishedEntryExpiresAfterTTL(t *testing.T) {
	registry := newExecSessionRegistry()
	key := execSessionKey{vmName: "vm", sessionID: "session"}
	session := newManualExecSessionForTest(key, registry)
	session.exitTTL = 10 * time.Millisecond
	registry.sessions[key] = session

	session.markFinished()

	require.Eventually(t, func() bool {
		_, ok := registry.get(key)

		return !ok
	}, time.Second, 10*time.Millisecond)
}
