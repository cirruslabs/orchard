package vmmanager

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avast/retry-go"
	"github.com/cirruslabs/chacha/pkg/localnetworkhelper"
	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/cirruslabs/orchard/internal/worker/tart"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

var ErrVMFailed = errors.New("VM failed")

type VM struct {
	onDiskName ondiskname.OnDiskName
	Resource   v1.VM
	logger     *zap.SugaredLogger

	// cloned state allows us to prevent a superfluous "tart delete" of a non-existent VM
	cloned atomic.Bool

	// started state allows us to only set VM's "status" field in the API
	// to "running" when we're indeed about to start the VM
	started atomic.Bool

	// stopping state allows us to catch unexpected
	// "tart run" terminations more correctly
	stopping atomic.Bool

	// Image FQN feature, see https://github.com/cirruslabs/orchard/issues/164
	imageFQN atomic.Pointer[string]

	status atomic.Pointer[string]
	err    atomic.Pointer[error]

	ctx    context.Context
	cancel context.CancelFunc

	wg *sync.WaitGroup

	localNetworkHelper *localnetworkhelper.LocalNetworkHelper
}

func NewVM(
	vmResource v1.VM,
	eventStreamer *client.EventStreamer,
	vmPullTimeHistogram metric.Float64Histogram,
	localNetworkHelper *localnetworkhelper.LocalNetworkHelper,
	logger *zap.SugaredLogger,
) *VM {
	vmContext, vmContextCancel := context.WithCancel(context.Background())

	vm := &VM{
		onDiskName: ondiskname.NewFromResource(vmResource),
		Resource:   vmResource,
		logger: logger.With(
			"vm_uid", vmResource.UID,
			"vm_name", vmResource.Name,
			"vm_restart_count", vmResource.RestartCount,
		),

		ctx:    vmContext,
		cancel: vmContextCancel,

		wg: &sync.WaitGroup{},

		localNetworkHelper: localNetworkHelper,
	}

	vm.wg.Add(1)

	go func() {
		defer vm.wg.Done()

		if vmResource.ImagePullPolicy == v1.ImagePullPolicyAlways {
			vm.setStatus("pulling VM image...")

			pullStartedAt := time.Now()

			_, _, err := tart.Tart(vm.ctx, vm.logger, "pull", vm.Resource.Image)
			if err != nil {
				select {
				case <-vm.ctx.Done():
					// Do not return an error because it's the user's intent to cancel this VM operation
				default:
					vm.setErr(fmt.Errorf("failed to pull the VM: %w", err))
				}

				return
			}

			vmPullTimeHistogram.Record(vm.ctx, time.Since(pullStartedAt).Seconds(), metric.WithAttributes(
				attribute.String("worker", vm.Resource.Worker),
				attribute.String("image", vm.Resource.Image),
			))
		}

		if err := vm.cloneAndConfigure(vm.ctx); err != nil {
			select {
			case <-vm.ctx.Done():
				// Do not return an error because it's the user's intent to cancel this VM operation
			default:
				vm.setErr(fmt.Errorf("failed to clone the VM: %w", err))
			}

			return
		}

		vm.cloned.Store(true)

		// Launch the startup script goroutine as close as possible
		// to the VM startup (below) to avoid "tart ip" timing out
		if vm.Resource.StartupScript != nil {
			vm.setStatus("VM started, running startup script...")

			go vm.runScript(vm.Resource.StartupScript, eventStreamer)
		} else {
			vm.setStatus("VM started")
		}

		vm.started.Store(true)

		if err := vm.run(vm.ctx); err != nil {
			select {
			case <-vm.ctx.Done():
				// Do not return an error because it's the user's intent to cancel this VM
			default:
				vm.setErr(fmt.Errorf("%w: %v", ErrVMFailed, err))
			}

			return
		}

		select {
		case <-vm.ctx.Done():
			// Do not return an error because it's the user's intent to cancel this VM
		default:
			if !vm.stopping.Load() {
				vm.setErr(fmt.Errorf("%w: VM exited unexpectedly", ErrVMFailed))
			}
		}
	}()

	return vm
}

func (vm *VM) OnDiskName() ondiskname.OnDiskName {
	return vm.onDiskName
}

func (vm *VM) Started() bool {
	return vm.started.Load()
}

func (vm *VM) ImageFQN() *string {
	return vm.imageFQN.Load()
}

func (vm *VM) id() string {
	return vm.onDiskName.String()
}

func (vm *VM) Status() string {
	status := vm.status.Load()

	if status != nil {
		return *status
	}

	return ""
}

func (vm *VM) setStatus(status string) {
	vm.logger.Debugf(status)
	vm.status.Store(&status)
}

func (vm *VM) Err() error {
	if err := vm.err.Load(); err != nil {
		return *err
	}

	return nil
}

func (vm *VM) setErr(err error) {
	if vm.err.CompareAndSwap(nil, &err) {
		vm.logger.Error(err)
	}
}

func (vm *VM) cloneAndConfigure(ctx context.Context) error {
	vm.setStatus("cloning VM...")

	_, _, err := tart.Tart(ctx, vm.logger, "clone", vm.Resource.Image, vm.id())
	if err != nil {
		return err
	}

	// Image FQN feature, see https://github.com/cirruslabs/orchard/issues/164
	fqnRaw, _, err := tart.Tart(ctx, vm.logger, "fqn", vm.Resource.Image)
	if err == nil {
		fqn := strings.TrimSpace(fqnRaw)
		vm.imageFQN.Store(&fqn)
	}

	// Set memory
	vm.setStatus("configuring VM...")

	memory := vm.Resource.AssignedMemory

	if memory == 0 {
		memory = vm.Resource.Memory
	}

	if memory != 0 {
		_, _, err = tart.Tart(ctx, vm.logger, "set", "--memory",
			strconv.FormatUint(memory, 10), vm.id())
		if err != nil {
			return err
		}
	}

	// Set CPU
	cpu := vm.Resource.AssignedCPU

	if cpu == 0 {
		cpu = vm.Resource.CPU
	}

	if cpu != 0 {
		_, _, err = tart.Tart(ctx, vm.logger, "set", "--cpu",
			strconv.FormatUint(cpu, 10), vm.id())
		if err != nil {
			return err
		}
	}

	if diskSize := vm.Resource.DiskSize; diskSize != 0 {
		_, _, err = tart.Tart(ctx, vm.logger, "set", "--disk-size",
			strconv.FormatUint(diskSize, 10), vm.id())
		if err != nil {
			return err
		}
	}

	// Randomize VM's MAC-address, this is important when using shared (NAT) networking
	// with full /var/db/dhcpd_leases file (e.g. 256 entries) having an expired entry
	// for a MAC address used by some OCI image, for example:
	//
	// {
	//	name=adminsVlMachine
	//	ip_address=192.168.64.2
	//	hw_address=1,11:11:11:11:11:11
	//	identifier=1,11:11:11:11:11:11
	//	lease=0x1234
	//}
	//
	// The next VM to start with a MAC address 22:22:22:22:22:22 will assume that
	// 192.168.64.2 is free (because its lease expired a long time ago) and will
	// add a new entry using its MAC address and 192.168.64.2 to the
	// /var/db/dhcpd_leases and won't delete the old entry:
	//
	// {
	//	name=adminsVlMachine
	//	ip_address=192.168.64.2
	//	hw_address=1,11:11:11:11:11:11
	//	identifier=1,11:11:11:11:11:11
	//	lease=0x1234
	// }
	// {
	//	name=adminsVlMachine
	//	ip_address=192.168.64.2
	//	hw_address=1,22:22:22:22:22:22
	//	identifier=1,22:22:22:22:22:22
	//	lease=0x67ade532
	// }
	//
	// Afterward, when an OCI VM with MAC address 11:11:11:11:11:11 is cloned and run,
	// it will re-use the 192.168.64.2 entry instead of creating a new one, even through
	// its lease had already expired. The resulting /var/db/dhcpd_leases will look like this:
	//
	// {
	//	name=adminsVlMachine
	//	ip_address=192.168.64.2
	//	hw_address=1,11:11:11:11:11:11
	//	identifier=1,11:11:11:11:11:11
	//	lease=0x67ade5c6
	// }
	// {
	//	name=adminsVlMachine
	//	ip_address=192.168.64.2
	//	hw_address=1,22:22:22:22:22:22
	//	identifier=1,22:22:22:22:22:22
	//	lease=0x67ade532
	// }
	//
	// As a result, you will see two VMs with different MAC address using an identical
	// IP address 192.168.64.2.
	//
	// Another scenarion when this is important is when using bridged networking
	// to avoid collisions when cloning from an OCI image on multiple hosts[1].
	//
	// [1]: https://github.com/cirruslabs/orchard/issues/181
	_, _, err = tart.Tart(ctx, vm.logger, "set", "--random-mac", vm.id())
	if err != nil {
		return err
	}

	if vm.Resource.RandomSerial {
		_, _, err = tart.Tart(ctx, vm.logger, "set", "--random-serial", vm.id())
		if err != nil {
			return err
		}
	}

	return nil
}

func (vm *VM) run(ctx context.Context) error {
	var runArgs = []string{"run"}

	if vm.Resource.NetSoftnetDeprecated || vm.Resource.NetSoftnet {
		runArgs = append(runArgs, "--net-softnet")
	}
	if len(vm.Resource.NetSoftnetAllow) != 0 {
		runArgs = append(runArgs, "--net-softnet-allow", strings.Join(vm.Resource.NetSoftnetAllow, ","))
	}
	if len(vm.Resource.NetSoftnetBlock) != 0 {
		runArgs = append(runArgs, "--net-softnet-block", strings.Join(vm.Resource.NetSoftnetBlock, ","))
	}
	if vm.Resource.NetBridged != "" {
		runArgs = append(runArgs, fmt.Sprintf("--net-bridged=%s", vm.Resource.NetBridged))
	}

	if vm.Resource.Headless {
		runArgs = append(runArgs, "--no-graphics")
	}

	if vm.Resource.Nested {
		runArgs = append(runArgs, "--nested")
	}
	if vm.Resource.Suspendable {
		runArgs = append(runArgs, "--suspendable")
	}

	for _, hostDir := range vm.Resource.HostDirs {
		runArgs = append(runArgs, fmt.Sprintf("--dir=%s", hostDir.String()))
	}

	runArgs = append(runArgs, vm.id())
	_, _, err := tart.Tart(ctx, vm.logger, runArgs...)
	if err != nil {
		return err
	}

	return nil
}

func (vm *VM) IP(ctx context.Context) (string, error) {
	// Bridged networking is problematic, so try with
	// the agent resolver first using a small timeout
	if vm.Resource.NetBridged != "" {
		stdout, _, err := tart.Tart(ctx, vm.logger, "ip", "--wait", "5",
			"--resolver", "agent", vm.id())
		if err == nil {
			return strings.TrimSpace(stdout), nil
		}
	}

	args := []string{"ip", "--wait", "60"}

	if vm.Resource.NetBridged != "" {
		args = append(args, "--resolver", "arp")
	}

	args = append(args, vm.id())

	stdout, _, err := tart.Tart(ctx, vm.logger, args...)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (vm *VM) Stop() {
	vm.logger.Debugf("stopping VM")

	vm.stopping.Store(true)

	// Try to gracefully terminate the VM
	_, _, _ = tart.Tart(context.Background(), zap.NewNop().Sugar(), "stop", "--timeout", "5", vm.id())

	// Terminate the VM goroutine ("tart pull", "tart clone", "tart run", etc.) via the context
	vm.cancel()
	vm.wg.Wait()

	vm.logger.Debugf("VM stopped")
}

func (vm *VM) Delete() error {
	if !vm.cloned.Load() {
		return nil
	}

	vm.logger.Debugf("deleting VM")

	_, _, err := tart.Tart(context.Background(), vm.logger, "delete", vm.id())
	if err != nil {
		return fmt.Errorf("%w: failed to delete VM: %v", ErrVMFailed, err)
	}

	vm.logger.Debugf("deleted VM")

	return nil
}

func (vm *VM) shell(
	ctx context.Context,
	sshUser string,
	sshPassword string,
	script string,
	env map[string]string,
	consumeLine func(line string),
) error {
	var sess *ssh.Session

	// Set default user and password if not provided
	if sshUser == "" && sshPassword == "" {
		sshUser = "admin"
		sshPassword = "admin"
	}

	// Configure SSH client
	sshConfig := &ssh.ClientConfig{
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(sshPassword),
		},
	}

	if err := retry.Do(func() error {
		ip, err := vm.IP(ctx)
		if err != nil {
			return fmt.Errorf("failed to get VM's IP: %w", err)
		}

		addr := ip + ":22"

		dialCtx, dialCtxCancel := context.WithTimeout(ctx, 5*time.Second)
		defer dialCtxCancel()

		var netConn net.Conn

		if vm.localNetworkHelper != nil {
			netConn, err = vm.localNetworkHelper.PrivilegedDialContext(dialCtx, "tcp", addr)
		} else {
			dialer := net.Dialer{}

			netConn, err = dialer.DialContext(dialCtx, "tcp", addr)
		}
		if err != nil {
			return fmt.Errorf("failed to dial %s: %w", addr, err)
		}

		sshConn, chans, reqs, err := ssh.NewClientConn(netConn, addr, sshConfig)
		if err != nil {
			return fmt.Errorf("SSH handshake with %s failed: %w", addr, err)
		}

		sshClient := ssh.NewClient(sshConn, chans, reqs)

		sess, err = sshClient.NewSession()
		if err != nil {
			return fmt.Errorf("failed to open an SSH session on %s: %w", addr, err)
		}

		return nil
	}, retry.Context(ctx), retry.OnRetry(func(n uint, err error) {
		consumeLine(fmt.Sprintf("attempt %d to establish SSH connection failed: %v", n, err))
	})); err != nil {
		return fmt.Errorf("failed to establish SSH connection: %w", err)
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

func (vm *VM) runScript(script *v1.VMScript, eventStreamer *client.EventStreamer) {
	if eventStreamer != nil {
		defer func() {
			if err := eventStreamer.Close(); err != nil {
				vm.logger.Errorf("errored during streaming events for startup script: %v", err)
			}
		}()
	}

	consumeLine := func(line string) {
		if eventStreamer == nil {
			return
		}

		eventStreamer.Stream(v1.Event{
			Kind:      v1.EventKindLogLine,
			Timestamp: time.Now().Unix(),
			Payload:   line,
		})
	}

	err := vm.shell(vm.ctx, vm.Resource.Username, vm.Resource.Password,
		script.ScriptContent, script.Env, consumeLine)
	if err != nil {
		vm.setErr(fmt.Errorf("%w: failed to run startup script: %v", ErrVMFailed, err))
	}
}
