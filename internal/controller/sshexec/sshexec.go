package sshexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/cirruslabs/orchard/internal/execstream"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Options struct {
	Interactive bool
	TTY         bool
	Rows        uint32
	Cols        uint32
	Env         map[string]string
	Workdir     string
}

type Client struct {
	sshClient *ssh.Client
	done      chan struct{}
	waitErr   error
	waitMu    sync.Mutex
}

type Exec struct {
	ownedClient *Client
	sshSession  *ssh.Session
	stdout      io.Reader
	stderr      io.Reader
	stdin       io.WriteCloser
	stdinReader *io.PipeReader
	tty         bool
}

func NewClient(netConn net.Conn, user string, password string) (*Client, error) {
	// Establish an SSH connection
	sshConn, sshChans, sshReqs, err := ssh.NewClientConn(netConn, "", &ssh.ClientConfig{
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create an SSH connection: %w", err)
	}

	client := &Client{
		sshClient: ssh.NewClient(sshConn, sshChans, sshReqs),
		done:      make(chan struct{}),
	}

	go func() {
		err := client.sshClient.Wait()

		client.waitMu.Lock()
		client.waitErr = err
		client.waitMu.Unlock()

		close(client.done)
	}()

	return client, nil
}

func New(netConn net.Conn, user string, password string, options Options) (*Exec, error) {
	client, err := NewClient(netConn, user, password)
	if err != nil {
		return nil, err
	}

	exec, err := client.NewExec(options)
	if err != nil {
		_ = client.Close()

		return nil, err
	}

	exec.ownedClient = client

	return exec, nil
}

func (client *Client) NewExec(options Options) (*Exec, error) {
	// Create a new SSH session
	sshSession, err := client.sshClient.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create an SSH session: %w", err)
	}

	exec := &Exec{
		sshSession: sshSession,
		tty:        options.TTY,
	}

	if options.Interactive || options.TTY {
		stdinReader, stdinWriter := io.Pipe()
		sshSession.Stdin = stdinReader
		exec.stdinReader = stdinReader
		exec.stdin = stdinWriter
	}

	if options.TTY {
		if err := sshSession.RequestPty(
			"xterm-256color",
			int(options.Rows),
			int(options.Cols),
			ssh.TerminalModes{},
		); err != nil {
			_ = sshSession.Close()

			return nil, fmt.Errorf("failed to request PTY for the SSH session: %w", err)
		}
	}

	exec.stdout, err = sshSession.StdoutPipe()
	if err != nil {
		_ = sshSession.Close()

		return nil, fmt.Errorf("failed to create standard output pipe "+
			"for the SSH session: %w", err)
	}

	exec.stderr, err = sshSession.StderrPipe()
	if err != nil {
		_ = sshSession.Close()

		return nil, fmt.Errorf("failed to create standard error pipe "+
			"for the SSH session: %w", err)
	}

	return exec, nil
}

func (client *Client) Keepalive() error {
	_, _, err := client.sshClient.SendRequest("keepalive@openssh.com", true, nil)

	return err
}

func (client *Client) Done() <-chan struct{} {
	return client.done
}

func (client *Client) Err() error {
	<-client.done

	client.waitMu.Lock()
	defer client.waitMu.Unlock()

	return client.waitErr
}

func (client *Client) Close() error {
	return client.sshClient.Close()
}

func (client *Client) ShouldRecreateAfter(err error) bool {
	select {
	case <-client.done:
		return true
	default:
	}

	return errors.Is(err, io.EOF) ||
		errors.Is(err, net.ErrClosed) ||
		strings.Contains(err.Error(), "use of closed network connection")
}

func (exec *Exec) Stdin() io.WriteCloser {
	return exec.stdin
}

func CommandWithOptions(command string, options Options) (string, error) {
	if strings.ContainsRune(options.Workdir, '\x00') {
		return "", errors.New("working directory contains NUL byte")
	}

	keys := make([]string, 0, len(options.Env))
	for key, value := range options.Env {
		if !envNamePattern.MatchString(key) {
			return "", fmt.Errorf("invalid environment variable name %q", key)
		}

		if strings.ContainsRune(value, '\x00') {
			return "", fmt.Errorf("environment variable %q contains NUL byte", key)
		}

		keys = append(keys, key)
	}
	sort.Strings(keys)

	if command == "" {
		return command, nil
	}

	if options.Workdir == "" && len(keys) == 0 {
		return command, nil
	}

	var builder strings.Builder
	if options.Workdir != "" {
		builder.WriteString("cd ")
		builder.WriteString(shellQuote(options.Workdir))
		builder.WriteString(" || exit $?\n")
	}
	for _, key := range keys {
		builder.WriteString("export ")
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(shellQuote(options.Env[key]))
		builder.WriteByte('\n')
	}
	builder.WriteString(command)

	return builder.String(), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (exec *Exec) Resize(rows uint32, cols uint32) error {
	if !exec.tty {
		return errors.New("this exec session does not have a TTY")
	}

	return exec.sshSession.WindowChange(int(rows), int(cols))
}

func (exec *Exec) Run(
	ctx context.Context,
	command string,
	outgoingFrames chan<- *execstream.Frame,
) error {
	if exec.stdinReader != nil {
		defer func() {
			_ = exec.stdinReader.Close()
		}()
	}

	if err := exec.sshSession.Start(command); err != nil {
		return fmt.Errorf("failed to start command %q: %w", command, err)
	}

	// Read bytes from standard output and standard error and stream them as frames
	ioGroup, ioGroupCtx := errgroup.WithContext(ctx)

	ioGroup.Go(func() error {
		return ioStreamReader(ioGroupCtx, exec.stdout, execstream.FrameTypeStdout, outgoingFrames)
	})
	ioGroup.Go(func() error {
		return ioStreamReader(ioGroupCtx, exec.stderr, execstream.FrameTypeStderr, outgoingFrames)
	})

	sshWaitErrCh := make(chan error, 1)
	go func() {
		sshWaitErrCh <- exec.sshSession.Wait()
	}()

	// Wait for SSH command terminate while respecting context
	var sshWaitErr error

	select {
	case sshWaitErr = <-sshWaitErrCh:
		// Proceed
	case <-ctx.Done():
		return ctx.Err()
	}

	// Wait for the I/O to complete, otherwise we may
	// miss some bits of the command's output/error
	if err := ioGroup.Wait(); err != nil {
		return err
	}

	// Post an exit event
	exitFrame := &execstream.Frame{
		Type: execstream.FrameTypeExit,
		Exit: &execstream.Exit{
			Code: 0,
		},
	}

	if sshWaitErr != nil {
		var sshExitError *ssh.ExitError
		if errors.As(sshWaitErr, &sshExitError) {
			exitFrame.Exit.Code = int32(sshExitError.ExitStatus())
		} else {
			return fmt.Errorf("failed to execute command %q: %w", command, sshWaitErr)
		}
	}

	select {
	case outgoingFrames <- exitFrame:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func ioStreamReader(
	ctx context.Context,
	r io.Reader,
	frameType execstream.FrameType,
	ch chan<- *execstream.Frame,
) error {
	buf := make([]byte, 4096)

	for {
		n, err := r.Read(buf)

		if n > 0 {
			frame := &execstream.Frame{
				Type: frameType,
				Data: slices.Clone(buf[:n]),
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case ch <- frame:
				// Proceed
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return err
		}
	}
}

func (exec *Exec) Close() error {
	if exec.stdin != nil {
		_ = exec.stdin.Close()
	}
	if exec.stdinReader != nil {
		_ = exec.stdinReader.Close()
	}

	if err := exec.sshSession.Close(); err != nil {
		if exec.ownedClient != nil {
			_ = exec.ownedClient.Close()
		}

		return err
	}

	if exec.ownedClient != nil {
		return exec.ownedClient.Close()
	}

	return nil
}
