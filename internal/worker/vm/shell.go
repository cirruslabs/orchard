package vm

import (
	"bufio"
	"context"
	"fmt"
	"github.com/avast/retry-go/v4"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

func (vm *VM) shell(
	ctx context.Context,
	sshUser string,
	sshPassword string,
	script string,
	env map[string]string,
	consumeLine func(line string),
) error {
	ip, err := vm.IP(ctx)
	if err != nil {
		return fmt.Errorf("%w to get IP", ErrVMFailed)
	}

	var netConn net.Conn

	addr := ip + ":22"

	if err := retry.Do(func() error {
		dialer := net.Dialer{}

		netConn, err = dialer.DialContext(ctx, "tcp", addr)

		return err
	}, retry.Context(ctx)); err != nil {
		return fmt.Errorf("%w to dial: %v", ErrVMFailed, err)
	}

	// set default user and password if not provided
	if sshUser == "" && sshPassword == "" {
		sshUser = "admin"
		sshPassword = "admin"
	}

	sshConfig := &ssh.ClientConfig{
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(sshPassword),
		},
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(netConn, addr, sshConfig)
	if err != nil {
		return fmt.Errorf("%w to connect via SSH: %v", ErrVMFailed, err)
	}
	cli := ssh.NewClient(sshConn, chans, reqs)

	sess, err := cli.NewSession()
	if err != nil {
		return fmt.Errorf("%w: failed to open SSH session: %v", ErrVMFailed, err)
	}

	// Log output from the virtual machine
	stdout, err := sess.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%w: while opening stdout pipe: %v", ErrVMFailed, err)
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		return fmt.Errorf("%w: while opening stderr pipe: %v", ErrVMFailed, err)
	}
	var outputReaderWG sync.WaitGroup
	outputReaderWG.Add(1)
	go func() {
		output := io.MultiReader(stdout, stderr)

		scanner := bufio.NewScanner(output)

		for scanner.Scan() {
			consumeLine(scanner.Text())
		}
		outputReaderWG.Done()
	}()

	stdinBuf, err := sess.StdinPipe()
	if err != nil {
		return fmt.Errorf("%w: while opening stdin pipe: %v", ErrVMFailed, err)
	}

	// start a login shell so all the customization from ~/.zprofile will be picked up
	err = sess.Shell()
	if err != nil {
		return fmt.Errorf("%w: failed to start a shell: %v", ErrVMFailed, err)
	}

	var scriptBuilder strings.Builder

	scriptBuilder.WriteString("set -e\n")
	// don't use sess.Setenv since it requires non-default SSH server configuration
	for key, value := range env {
		scriptBuilder.WriteString("export " + key + "=\"" + value + "\"\n")
	}
	scriptBuilder.WriteString(script)
	scriptBuilder.WriteString("\nexit\n")

	_, err = stdinBuf.Write([]byte(scriptBuilder.String()))
	if err != nil {
		return fmt.Errorf("%w: failed to start script: %v", ErrVMFailed, err)
	}
	outputReaderWG.Wait()
	return sess.Wait()
}

func (vm *VM) shellHelper(ctx context.Context, vmResource v1.VM, script *v1.VMScript) error {
	consumeLineFunc := func(line string) {
		if vm.eventStreamer == nil {
			return
		}

		vm.eventStreamer.Stream(v1.Event{
			Kind:      v1.EventKindLogLine,
			Timestamp: time.Now().Unix(),
			Payload:   line,
		})
	}

	err := vm.shell(ctx, vmResource.Username, vmResource.Password,
		script.ScriptContent, script.Env, consumeLineFunc)
	if err != nil {
		vm.logger.Errorf("failed to run script for VM: %s", err.Error())
	}

	return err
}
