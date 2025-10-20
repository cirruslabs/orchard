package exec

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cirruslabs/orchard/internal/execstream"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	vmTimeout     time.Duration
	vmInteractive bool
	vmTTY         bool
)

func newExecVMCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "vm VM_NAME COMMAND [ARGS...]",
		Short: "Execute a command inside the VM",
		Args:  cobra.MinimumNArgs(2),
		RunE:  runExecVM,
	}

	command.Flags().DurationVarP(&vmTimeout, "timeout", "w", 60*time.Second,
		"time to wait for the VM to reach running state")
	command.Flags().BoolVarP(&vmInteractive, "interactive", "i", false,
		"attach local standard input to the remote command")
	command.Flags().BoolVarP(&vmTTY, "tty", "t", false,
		"allocate a pseudo-terminal on the remote end")

	return command
}

func runExecVM(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	name := args[0]
	commandArgs := args[1:]

	client, err := client.New()
	if err != nil {
		return err
	}

	ctx := cmd.Context()

	if err := waitForVMRunning(ctx, client, name, vmTimeout); err != nil {
		return err
	}

	rows, cols := uint32(0), uint32(0)
	if vmTTY {
		width, height, err := term.GetSize(int(os.Stdout.Fd()))
		if err == nil {
			cols = uint32(width)
			rows = uint32(height)
		}
	}

	interactive := vmInteractive || vmTTY

	waitSeconds := uint16(vmTimeout / time.Second)
	if waitSeconds == 0 {
		waitSeconds = 1
	}

	conn, err := client.VMs().Exec(ctx, name, commandArgs, interactive, vmTTY, rows, cols, waitSeconds)
	if err != nil {
		return fmt.Errorf("failed to start exec session: %w", err)
	}
	defer conn.Close()

	decoder := execstream.NewDecoder(conn)
	encoder := execstream.NewEncoder(conn)

	stdinCh := make(chan error, 1)
	resizeCh := make(chan error, 1)

	if vmInteractive || vmTTY {
		if vmTTY {
			stdinFD := int(os.Stdin.Fd())
			state, err := term.MakeRaw(stdinFD)
			if err != nil {
				return fmt.Errorf("failed to put terminal into raw mode: %w", err)
			}
			defer func() {
				_ = term.Restore(stdinFD, state)
			}()

			go monitorTerminalResize(ctx, encoder, resizeCh)
		}

		go streamStdin(ctx, encoder, stdinCh)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	var exitCode int32

loop:
	for {
		var frame execstream.Frame

		err := execstream.ReadFrame(decoder, &frame)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break loop
			}
			return fmt.Errorf("exec session read failed: %w", err)
		}

		switch frame.Type {
		case execstream.FrameTypeStdout:
			if len(frame.Data) > 0 {
				if _, err := os.Stdout.Write(frame.Data); err != nil {
					return err
				}
			}
		case execstream.FrameTypeStderr:
			if len(frame.Data) > 0 {
				if vmTTY {
					if _, err := os.Stdout.Write(frame.Data); err != nil {
						return err
					}
				} else {
					if _, err := os.Stderr.Write(frame.Data); err != nil {
						return err
					}
				}
			}
		case execstream.FrameTypeExit:
			if frame.Exit != nil {
				exitCode = frame.Exit.Code
			}
			break loop
		case execstream.FrameTypeError:
			return fmt.Errorf("exec error: %s", frame.Error)
		}
	}

	select {
	case err := <-stdinCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	default:
	}

	select {
	case err := <-resizeCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	default:
	}

	if exitCode != 0 {
		os.Exit(int(exitCode))
	}

	return nil
}

func waitForVMRunning(ctx context.Context, client *client.Client, name string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = time.Second
	}

	deadline := time.Now().Add(timeout)

	for {
		vm, err := client.VMs().Get(ctx, name)
		if err != nil {
			return err
		}

		switch vm.Status {
		case v1.VMStatusRunning:
			return nil
		case v1.VMStatusFailed:
			return fmt.Errorf("VM %s is in failed state: %s", name, vm.StatusMessage)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("VM %s did not reach running state within %s", name, timeout)
		}

		select {
		case <-time.After(1 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func streamStdin(ctx context.Context, encoder *json.Encoder, errCh chan<- error) {
	reader := bufio.NewReader(os.Stdin)

	for {
		select {
		case <-ctx.Done():
			errCh <- ctx.Err()
			return
		default:
		}

		buf := make([]byte, 4096)
		n, err := reader.Read(buf)
		if n > 0 {
			if err := execstream.WriteFrame(encoder, &execstream.Frame{
				Type: execstream.FrameTypeStdin,
				Data: buf[:n],
			}); err != nil {
				errCh <- err
				return
			}
		}

		if errors.Is(err, io.EOF) {
			execstream.WriteFrame(encoder, &execstream.Frame{Type: execstream.FrameTypeStdin})
			errCh <- nil
			return
		}

		if err != nil {
			errCh <- err
			return
		}
	}
}

func monitorTerminalResize(ctx context.Context, encoder *json.Encoder, errCh chan<- error) {
	stdoutFD := int(os.Stdout.Fd())
	prevWidth, prevHeight, err := term.GetSize(stdoutFD)
	if err != nil {
		errCh <- err
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			errCh <- ctx.Err()
			return
		case <-ticker.C:
			width, height, err := term.GetSize(stdoutFD)
			if err != nil {
				errCh <- err
				return
			}

			if width == prevWidth && height == prevHeight {
				continue
			}

			if err := execstream.WriteFrame(encoder, &execstream.Frame{
				Type: execstream.FrameTypeResize,
				Terminal: &execstream.TerminalSize{
					Rows: uint32(height),
					Cols: uint32(width),
				},
			}); err != nil {
				errCh <- err
				return
			}

			prevWidth = width
			prevHeight = height
		}
	}
}
