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
var wait uint16

func newSSHVMCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "vm VM_NAME [COMMAND]",
		Short: "SSH into the VM",
		Args:  cobra.RangeArgs(1, 2),
		RunE:  runSSHVM,
	}

	command.PersistentFlags().StringVarP(&username, "username", "u", "",
		"SSH username")
	command.PersistentFlags().StringVarP(&password, "password", "p", "",
		"SSH password")
	command.PersistentFlags().Uint16VarP(&wait, "wait", "t", 60,
		"Amount of seconds to wait for the VM to start running if it's not running already")

	return command
}

func runSSHVM(cmd *cobra.Command, args []string) error {
	// Required NAME argument
	name := args[0]

	// Optional [COMMAND] argument
	var command string

	if len(args) > 1 {
		command = args[1]
	}

	client, err := client.New()
	if err != nil {
		return err
	}

	wsConn, err := client.VMs().PortForward(cmd.Context(), name, 22, wait)
	if err != nil {
		return fmt.Errorf("%w: failed setup port-forwarding to the VM %q: %v", ErrFailed, name, err)
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
	defer func() {
		_ = sshSess.Close()
	}()

	if command != "" {
		sshSess.Stdout = os.Stdout
		sshSess.Stderr = os.Stderr
		sshSess.Stdin = os.Stdin

		return sshSess.Run(command)
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
	if err == nil && vm.Username != "" && vm.Password != "" {
		return vm.Username, vm.Password
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "failed to retrieve VM %s's credentials from the API server: %v\n",
			vmName, err)
	}

	// Fall back
	_, _ = fmt.Fprintf(os.Stderr, "no credentials specified or found, "+
		"trying default admin:admin credentials...\n")

	return "admin", "admin"
}
