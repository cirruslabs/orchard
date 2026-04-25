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

	"github.com/cirruslabs/orchard/internal/execstream"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Exec struct {
	sshClient  *ssh.Client
	sshSession *ssh.Session
	stdout     io.Reader
	stderr     io.Reader
	stdin      io.WriteCloser
}

func New(netConn net.Conn, user string, password string, stdin bool) (*Exec, error) {
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

	sshClient := ssh.NewClient(sshConn, sshChans, sshReqs)

	// Create a new SSH session
	sshSession, err := sshClient.NewSession()
	if err != nil {
		_ = sshClient.Close()

		return nil, fmt.Errorf("failed to create an SSH session: %w", err)
	}

	exec := &Exec{
		sshClient:  sshClient,
		sshSession: sshSession,
	}

	if stdin {
		exec.stdin, err = sshSession.StdinPipe()
		if err != nil {
			_ = sshSession.Close()
			_ = sshClient.Close()

			return nil, fmt.Errorf("failed to create standard input pipe "+
				"for the SSH session: %w", err)
		}
	}

	exec.stdout, err = sshSession.StdoutPipe()
	if err != nil {
		_ = sshSession.Close()
		_ = sshClient.Close()

		return nil, fmt.Errorf("failed to create standard output pipe "+
			"for the SSH session: %w", err)
	}

	exec.stderr, err = sshSession.StderrPipe()
	if err != nil {
		_ = sshSession.Close()
		_ = sshClient.Close()

		return nil, fmt.Errorf("failed to create standard error pipe "+
			"for the SSH session: %w", err)
	}

	return exec, nil
}

func (exec *Exec) Stdin() io.WriteCloser {
	return exec.stdin
}

func CommandWithEnv(command string, env map[string]string) (string, error) {
	if len(env) == 0 {
		return command, nil
	}

	keys := make([]string, 0, len(env))
	for key, value := range env {
		if !envNamePattern.MatchString(key) {
			return "", fmt.Errorf("invalid environment variable name %q", key)
		}

		if strings.ContainsRune(value, '\x00') {
			return "", fmt.Errorf("environment variable %q contains NUL byte", key)
		}

		keys = append(keys, key)
	}

	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString("export ")
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(shellQuote(env[key]))
		builder.WriteByte('\n')
	}
	builder.WriteString(command)

	return builder.String(), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (exec *Exec) Run(
	ctx context.Context,
	command string,
	outgoingFrames chan<- *execstream.Frame,
) error {
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
	if err := exec.sshSession.Close(); err != nil {
		_ = exec.sshClient.Close()

		return err
	}

	return exec.sshClient.Close()
}
