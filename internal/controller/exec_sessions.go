package controller

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/cirruslabs/orchard/internal/execstream"
)

const execSessionReplayBufferBytes = 4 * 1024 * 1024

type sshExecRunner interface {
	Stdin() io.WriteCloser
	Run(ctx context.Context, command string, outgoingFrames chan<- *execstream.Frame) error
	Close() error
}

type execSessionKey struct {
	vmName    string
	sessionID string
}

type execSessionCreation struct {
	done    chan struct{}
	session *execSession
	err     error
}

type execSessionRegistry struct {
	mu       sync.Mutex
	sessions map[execSessionKey]*execSession
	creating map[execSessionKey]*execSessionCreation
}

func newExecSessionRegistry() *execSessionRegistry {
	return &execSessionRegistry{
		sessions: map[execSessionKey]*execSession{},
		creating: map[execSessionKey]*execSessionCreation{},
	}
}

func (registry *execSessionRegistry) get(key execSessionKey) (*execSession, bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	session, ok := registry.sessions[key]

	return session, ok
}

func (registry *execSessionRegistry) getOrCreate(
	ctx context.Context,
	key execSessionKey,
	create func() (*execSession, error),
) (*execSession, bool, error) {
	for {
		registry.mu.Lock()

		if session, ok := registry.sessions[key]; ok {
			registry.mu.Unlock()

			return session, false, nil
		}

		if creation, ok := registry.creating[key]; ok {
			registry.mu.Unlock()

			select {
			case <-ctx.Done():
				return nil, false, ctx.Err()
			case <-creation.done:
				if creation.err != nil {
					return nil, false, creation.err
				}

				return creation.session, false, nil
			}
		}

		creation := &execSessionCreation{done: make(chan struct{})}
		registry.creating[key] = creation
		registry.mu.Unlock()

		session, err := create()

		registry.mu.Lock()
		delete(registry.creating, key)
		if err == nil {
			registry.sessions[key] = session
		}
		creation.session = session
		creation.err = err
		close(creation.done)
		registry.mu.Unlock()

		return session, true, err
	}
}

func (registry *execSessionRegistry) remove(key execSessionKey, expected *execSession) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if registry.sessions[key] == expected {
		delete(registry.sessions, key)
	}
}

func (registry *execSessionRegistry) closeAll() {
	registry.mu.Lock()
	sessions := make([]*execSession, 0, len(registry.sessions))
	for _, session := range registry.sessions {
		sessions = append(sessions, session)
	}
	registry.mu.Unlock()

	for _, session := range sessions {
		session.close()
	}
}

type execReplayFrame struct {
	frame *execstream.Frame
	size  int
}

type execSessionSubscriber struct {
	frames chan *execstream.Frame
}

func newExecSessionSubscriber() *execSessionSubscriber {
	return &execSessionSubscriber{
		frames: make(chan *execstream.Frame, 128),
	}
}

func (subscriber *execSessionSubscriber) enqueue(frame *execstream.Frame) bool {
	select {
	case subscriber.frames <- cloneExecFrame(frame):
		return true
	default:
		return false
	}
}

type execSession struct {
	key       execSessionKey
	command   string
	exec      sshExecRunner
	transport net.Conn
	registry  *execSessionRegistry
	exitTTL   time.Duration

	ctx    context.Context
	cancel context.CancelFunc

	mu             sync.Mutex
	stdin          io.WriteCloser
	stdinClosed    bool
	subscribers    map[*execSessionSubscriber]struct{}
	frames         []execReplayFrame
	bufferBytes    int
	nextWatermark  uint64
	ackedWatermark uint64
	finished       bool
	closed         bool
	expiryTimer    *time.Timer

	done     chan struct{}
	doneOnce sync.Once
}

func newExecSession(
	key execSessionKey,
	command string,
	exec sshExecRunner,
	transport net.Conn,
	registry *execSessionRegistry,
	exitTTL time.Duration,
) *execSession {
	ctx, cancel := context.WithCancel(context.Background())

	session := &execSession{
		key:         key,
		command:     command,
		exec:        exec,
		transport:   transport,
		registry:    registry,
		exitTTL:     exitTTL,
		ctx:         ctx,
		cancel:      cancel,
		stdin:       exec.Stdin(),
		subscribers: map[*execSessionSubscriber]struct{}{},
		done:        make(chan struct{}),
	}

	go session.run()

	return session
}

func (session *execSession) commandMatches(command string) bool {
	return command == "" || session.command == command
}

func (session *execSession) attach() (*execSessionSubscriber, error) {
	session.mu.Lock()
	defer session.mu.Unlock()

	if session.closed {
		return nil, errors.New("exec session is closed")
	}

	subscriber := newExecSessionSubscriber()
	session.subscribers[subscriber] = struct{}{}

	return subscriber, nil
}

func (session *execSession) detach(subscriber *execSessionSubscriber) {
	session.mu.Lock()
	defer session.mu.Unlock()

	session.detachLocked(subscriber)
}

func (session *execSession) detachLocked(subscriber *execSessionSubscriber) {
	if _, ok := session.subscribers[subscriber]; !ok {
		return
	}

	delete(session.subscribers, subscriber)
	close(subscriber.frames)
}

func (session *execSession) writeStdin(data []byte) error {
	session.mu.Lock()
	defer session.mu.Unlock()

	if session.stdin == nil || session.stdinClosed {
		return errors.New("this exec session has no stdin enabled or it is already closed")
	}

	if len(data) == 0 {
		if err := session.stdin.Close(); err != nil {
			return err
		}

		session.stdinClosed = true

		return nil
	}

	_, err := session.stdin.Write(data)

	return err
}

func (session *execSession) ack(watermark uint64) {
	session.mu.Lock()
	defer session.mu.Unlock()

	if watermark <= session.ackedWatermark {
		return
	}

	session.ackedWatermark = watermark
	session.trimAcknowledgedLocked()
}

func (session *execSession) sendHistory(
	subscriber *execSessionSubscriber,
	watermark uint64,
) {
	session.mu.Lock()
	defer session.mu.Unlock()

	if _, ok := session.subscribers[subscriber]; !ok {
		return
	}

	for _, record := range session.frames {
		if record.frame.Watermark <= watermark {
			continue
		}

		if !subscriber.enqueue(record.frame) {
			session.detachLocked(subscriber)

			return
		}
	}

	if !subscriber.enqueue(&execstream.Frame{
		Type:      execstream.FrameTypeNoMoreHistory,
		Watermark: session.nextWatermark,
	}) {
		session.detachLocked(subscriber)
	}
}

func (session *execSession) close() {
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()

		return
	}

	session.closed = true
	if session.expiryTimer != nil {
		session.expiryTimer.Stop()
		session.expiryTimer = nil
	}

	subscribers := make([]*execSessionSubscriber, 0, len(session.subscribers))
	for subscriber := range session.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	session.subscribers = map[*execSessionSubscriber]struct{}{}
	session.mu.Unlock()

	for _, subscriber := range subscribers {
		close(subscriber.frames)
	}

	session.cancel()
	_ = session.exec.Close()
	if session.transport != nil {
		_ = session.transport.Close()
	}
	session.registry.remove(session.key, session)
}

func (session *execSession) run() {
	outgoingFrames := make(chan *execstream.Frame)
	runErrCh := make(chan error, 1)

	go func() {
		runErrCh <- session.exec.Run(session.ctx, session.command, outgoingFrames)
		close(outgoingFrames)
	}()

	for frame := range outgoingFrames {
		session.recordFrame(frame)
	}

	runErr := <-runErrCh
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		session.recordFrame(&execstream.Frame{
			Type:  execstream.FrameTypeError,
			Error: runErr.Error(),
		})
	}

	session.markFinished()
}

func (session *execSession) recordFrame(frame *execstream.Frame) {
	session.mu.Lock()
	defer session.mu.Unlock()

	if session.closed {
		return
	}

	session.nextWatermark++
	frame = cloneExecFrame(frame)
	frame.Watermark = session.nextWatermark

	session.frames = append(session.frames, execReplayFrame{
		frame: frame,
		size:  execFrameSize(frame),
	})
	session.bufferBytes += execFrameSize(frame)
	session.trimAcknowledgedLocked()
	session.trimToLimitLocked()

	for subscriber := range session.subscribers {
		if subscriber.enqueue(frame) {
			continue
		}

		session.detachLocked(subscriber)
	}
}

func (session *execSession) markFinished() {
	session.mu.Lock()
	if session.finished {
		session.mu.Unlock()

		return
	}

	session.finished = true
	if !session.closed {
		session.expiryTimer = time.AfterFunc(session.exitTTL, session.expire)
	}

	subscribers := make([]*execSessionSubscriber, 0, len(session.subscribers))
	for subscriber := range session.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	session.subscribers = map[*execSessionSubscriber]struct{}{}
	session.mu.Unlock()

	for _, subscriber := range subscribers {
		close(subscriber.frames)
	}

	session.doneOnce.Do(func() {
		close(session.done)
	})
}

func (session *execSession) expire() {
	session.close()
}

func (session *execSession) trimAcknowledgedLocked() {
	for len(session.frames) > 0 && session.frames[0].frame.Watermark <= session.ackedWatermark {
		session.bufferBytes -= session.frames[0].size
		session.frames = session.frames[1:]
	}
}

func (session *execSession) trimToLimitLocked() {
	for session.bufferBytes > execSessionReplayBufferBytes && len(session.frames) > 0 {
		session.bufferBytes -= session.frames[0].size
		session.frames = session.frames[1:]
	}
}

func cloneExecFrame(frame *execstream.Frame) *execstream.Frame {
	if frame == nil {
		return nil
	}

	clone := *frame
	if frame.Data != nil {
		clone.Data = append([]byte(nil), frame.Data...)
	}
	if frame.Exit != nil {
		exit := *frame.Exit
		clone.Exit = &exit
	}

	return &clone
}

func execFrameSize(frame *execstream.Frame) int {
	if frame == nil {
		return 0
	}

	return len(frame.Data) + len(frame.Error) + 16
}
