package ssh

import (
	"context"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
	"io"
	"net"
	"os"
	"time"
)

var ErrFailed = errors.New("ssh command failed")

var username string
var password string

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "ssh VM_NAME",
		Short: "SSH into the VM",
		Args:  cobra.ExactArgs(1),
		RunE:  run,
	}

	command.PersistentFlags().StringVarP(&username, "username", "u", "",
		"SSH username")
	command.PersistentFlags().StringVarP(&password, "password", "p", "",
		"SSH password")

	return command
}

func run(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := client.New()
	if err != nil {
		return err
	}

	wsConn, err := client.VMs().PortForward(cmd.Context(), name, 22)
	if err != nil {
		fmt.Printf("failed to forward an SSH port to VM %s: %v\n", name, err)

		return err
	}
	defer wsConn.Close()

	username, password = ChooseUsernameAndPassword(cmd.Context(), client, name, username, password)

	sshConfig := &ssh.ClientConfig{
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(wsConn, "", sshConfig)
	if err != nil {
		return fmt.Errorf("%w: failed to establish an SSH connection: %v", ErrFailed, err)
	}

	sshClient := ssh.NewClient(sshConn, chans, reqs)

	sshSess, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("%w: failed to open an SSH session: %v", ErrFailed, err)
	}

	// Switch controlling terminal into raw mode,
	// otherwise ANSI escape sequences that allow
	// for cursor control and more wouldn't work
	terminalFd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(terminalFd)
	if err != nil {
		return fmt.Errorf("%w: failed to switch controlling terminal into raw mode: %v", ErrFailed, err)
	}
	defer func() {
		_ = term.Restore(terminalFd, state)
	}()

	width, height, err := term.GetSize(terminalFd)
	if err != nil {
		return err
	}

	if err := sshSess.RequestPty("xterm-256color", height, width, ssh.TerminalModes{}); err != nil {
		return fmt.Errorf("%w: failed to request the PTY from the SSH server: %v", ErrFailed, err)
	}

	sshSess.Stdout = os.Stdout
	sshSess.Stderr = os.Stderr
	sshSessStdinPipe, err := sshSess.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		_, _ = io.Copy(sshSessStdinPipe, os.Stdin)
		_ = sshSessStdinPipe.Close()
		_ = sshSess.Close()
	}()

	// Periodically adjust remote terminal size
	go func() {
		for {
			newWidth, newHeight, err := term.GetSize(terminalFd)
			if err != nil {
				return
			}

			if height == newHeight && width == newWidth {
				continue
			}

			if err := sshSess.WindowChange(newHeight, newWidth); err != nil {
				continue
			}

			height = newHeight
			width = newWidth

			time.Sleep(time.Second)
		}
	}()

	if err := sshSess.Shell(); err != nil {
		return err
	}

	sshErr := make(chan error)

	go func() {
		sshErr <- sshSess.Wait()
	}()

	select {
	case err := <-sshErr:
		return err
	case <-cmd.Context().Done():
		return cmd.Context().Err()
	}
}

func ChooseUsernameAndPassword(
	ctx context.Context,
	client *client.Client,
	vmName string,
	usernameFromUser string,
	passwordFromUser string,
) (string, string) {
	// User settings override everything
	if usernameFromUser != "" || passwordFromUser != "" {
		return usernameFromUser, passwordFromUser
	}

	// Try to get the credentials from the VM's object stored on controller
	vm, err := client.VMs().Get(ctx, vmName)
	if err != nil {
		fmt.Printf("failed to retrieve VM %s's credentials from the API server: %v\n", vmName, err)
	}

	if vm.Username != "" && vm.Password != "" {
		return vm.Username, vm.Password
	}

	// Fall back
	fmt.Println("no credentials specified or found, trying default admin:admin credentials...")

	return "admin", "admin"
}
