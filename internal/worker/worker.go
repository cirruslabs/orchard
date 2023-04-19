package worker

import (
	"context"
	"errors"
	"fmt"
	"github.com/avast/retry-go/v4"
	"github.com/cirruslabs/orchard/internal/worker/iokitregistry"
	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/cirruslabs/orchard/internal/worker/statematcher"
	"github.com/cirruslabs/orchard/internal/worker/tart"
	vmpkg "github.com/cirruslabs/orchard/internal/worker/vm"
	"github.com/cirruslabs/orchard/internal/worker/vmmanager"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
	"os"
	"time"
)

const pollInterval = 5 * time.Second

var ErrPollFailed = errors.New("failed to poll controller")
var ErrRegistrationFailed = errors.New("failed to register worker on the controller")

type Worker struct {
	name          string
	syncRequested chan bool
	vmm           *vmmanager.VMManager
	client        *client.Client
	resources     v1.Resources
	logger        *zap.SugaredLogger
}

func New(client *client.Client, opts ...Option) (*Worker, error) {
	worker := &Worker{
		client:        client,
		vmm:           vmmanager.New(),
		syncRequested: make(chan bool, 1),
	}

	// Apply options
	for _, opt := range opts {
		opt(worker)
	}

	// Apply defaults
	if worker.name == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, err
		}

		worker.name = hostname
	}

	defaultResources := v1.Resources{
		v1.ResourceTartVMs: 2,
	}
	worker.resources = defaultResources.Merged(worker.resources)

	if worker.logger == nil {
		worker.logger = zap.NewNop().Sugar()
	}

	return worker, nil
}

func (worker *Worker) Run(ctx context.Context) error {
	for {
		if err := worker.runNewSession(ctx); err != nil {
			return err
		}
	}
}

func (worker *Worker) Close() error {
	var result error

	for _, vm := range worker.vmm.List() {
		if err := vm.Close(); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result
}

func (worker *Worker) runNewSession(ctx context.Context) error {
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := worker.registerWorker(subCtx); err != nil {
		worker.logger.Warnf("failed to register worker: %v", err)

		return ErrRegistrationFailed
	}

	go func() {
		_ = retry.Do(func() error {
			return worker.watchRPC(subCtx)
		}, retry.OnRetry(func(n uint, err error) {
			worker.logger.Warnf("failed to watch RPC: %v", err)
		}), retry.Context(subCtx), retry.Attempts(0))
	}()

	// Sync on-disk VMs
	if err := worker.syncOnDiskVMs(ctx); err != nil {
		return err
	}

	for {
		if err := worker.updateWorker(ctx); err != nil {
			worker.logger.Errorf("failed to update worker resource: %v", err)

			return nil
		}

		if err := worker.syncVMs(subCtx); err != nil {
			worker.logger.Warnf("failed to sync VMs: %v", err)

			return nil
		}

		select {
		case <-worker.syncRequested:
		case <-time.After(pollInterval):
			// continue
		case <-subCtx.Done():
			return subCtx.Err()
		}
	}
}

func (worker *Worker) registerWorker(ctx context.Context) error {
	platformUUID, err := iokitregistry.PlatformUUID()
	if err != nil {
		return err
	}

	_, err = worker.client.Workers().Create(ctx, v1.Worker{
		Meta: v1.Meta{
			Name: worker.name,
		},
		Resources: worker.resources,
		LastSeen:  time.Now(),
		MachineID: platformUUID,
	})
	if err != nil {
		return err
	}

	worker.logger.Infof("registered worker %s", worker.name)

	return nil
}

func (worker *Worker) updateWorker(ctx context.Context) error {
	workerResource, err := worker.client.Workers().Get(ctx, worker.name)
	if err != nil {
		return fmt.Errorf("%w: failed to retrieve worker from the API: %v", ErrPollFailed, err)
	}

	worker.logger.Debugf("got worker from the API")

	workerResource.LastSeen = time.Now()

	if _, err := worker.client.Workers().Update(ctx, *workerResource); err != nil {
		return fmt.Errorf("%w: failed to update worker in the API: %v", ErrPollFailed, err)
	}

	worker.logger.Debugf("updated worker in the API")

	return nil
}

func (worker *Worker) syncVMs(ctx context.Context) error {
	remoteVMs, err := worker.client.VMs().FindForWorker(ctx, worker.name)
	if err != nil {
		return err
	}

	worker.logger.Infof("syncing %d remote VMs against %d local VMs...", len(remoteVMs), worker.vmm.Len())

	allUIDs := mapset.NewSet[string]()

	for _, remoteVM := range remoteVMs {
		allUIDs.Add(remoteVM.UID)
	}

	for _, localVM := range worker.vmm.List() {
		allUIDs.Add(localVM.Resource.UID)
	}

	getLocalVM := func(uid string) *vmpkg.VM {
		localVM, ok := worker.vmm.Get(v1.VM{UID: uid})
		if !ok {
			return nil
		}

		return localVM
	}

	getRemoteVM := func(uid string) *v1.VM {
		remoteVM, ok := remoteVMs[uid]
		if !ok {
			return nil
		}

		return &remoteVM
	}

	for uid := range allUIDs.Iter() {
		worker.syncVM(ctx, uid, getLocalVM(uid), getRemoteVM(uid))
	}

	return nil
}

func (worker *Worker) syncVM(ctx context.Context, uid string, localVM *vmpkg.VM, remoteVM *v1.VM) {
	logger := worker.logger.With("vm_uid", uid)

	logger.Debugf("comparing local VM %v with remote VM %v",
		localVMStateDescription(localVM), remoteVMStateDescription(remoteVM))

	rules := []statematcher.Rule[v1.VMStatus, vmpkg.State]{
		// Controller wants to start the VM
		{
			RemoteState: statematcher.Exact(v1.VMStatusPending),
			LocalState: statematcher.OneOf(
				statematcher.None[vmpkg.State](),
				statematcher.Exact(vmpkg.StateStopped),
			),
			Action: func() {
				vm, ok := worker.vmm.Get(*remoteVM)
				if !ok {
					vm = vmpkg.New(*remoteVM, worker.client.VMs().StreamEvents(remoteVM.Name), logger)
					worker.vmm.Put(vm)
				}

				vm.Action(vmpkg.ActionStart)
			},
		},
		// We have successfully started the VM, so we need to update the state on the controller
		{
			RemoteState: statematcher.Exact(v1.VMStatusPending),
			LocalState:  statematcher.Exact(vmpkg.StateStarted),
			Action: func() {
				worker.updateRemoteStatus(ctx, *remoteVM, v1.VMStatusRunning, "")
			},
		},
		// Controller wants to stop the VM
		{
			RemoteState: statematcher.Exact(v1.VMStatusStopping),
			LocalState:  statematcher.Exact(vmpkg.StateStarted),
			Action: func() {
				localVM.Action(vmpkg.ActionStop)
			},
		},
		// We have successfully stopped the VM, so we need to update the state on the controller
		{
			RemoteState: statematcher.Exact(v1.VMStatusStopping),
			LocalState:  statematcher.Exact(vmpkg.StateStopped),
			Action: func() {
				worker.updateRemoteStatus(ctx, *remoteVM, v1.VMStatusStopped, "")
			},
		},
		// Our VM has failed, but it still exists on the controller, so let the controller know that
		{
			RemoteState: statematcher.Some[v1.VMStatus](),
			LocalState:  statematcher.Exact(vmpkg.StateFailed),
			Action: func() {
				worker.updateRemoteStatus(ctx, *remoteVM, v1.VMStatusFailed, localVM.Err().Error())
			},
		},
		// VM that we have is removed from the controller, delete it
		{
			RemoteState: statematcher.None[v1.VMStatus](),
			LocalState:  statematcher.Not(vmpkg.StateDeleted),
			Action: func() {
				localVM.Action(vmpkg.ActionDelete)
			},
		},
		// We have a VM that has been successfully deleted, garbage-collect it from the VM manager
		{
			RemoteState: statematcher.Any[v1.VMStatus](),
			LocalState:  statematcher.Exact(vmpkg.StateDeleted),
			Action: func() {
				worker.vmm.Delete(localVM.Resource)
			},
		},
	}

	if rules := statematcher.Match(rules, remoteVMState(remoteVM), localVMState(localVM)); rules != nil {
		rules.Action()
	}
}

func (worker *Worker) syncOnDiskVMs(ctx context.Context) error {
	remoteVMs, err := worker.client.VMs().FindForWorker(ctx, worker.name)
	if err != nil {
		return err
	}

	worker.logger.Infof("syncing on-disk VMs...")

	vmInfos, err := tart.List(ctx, worker.logger)
	if err != nil {
		return err
	}

	for _, vmInfo := range vmInfos {
		if vmInfo.Running {
			continue
		}

		onDiskName, err := ondiskname.Parse(vmInfo.Name)
		if err != nil {
			if errors.Is(err, ondiskname.ErrNotManagedByOrchard) {
				continue
			}

			return err
		}

		remoteVM, ok := remoteVMs[onDiskName.UID]
		if !ok {
			// On-disk VM doesn't exist on the controller, delete it
			_, _, err := tart.Tart(ctx, worker.logger, "delete", vmInfo.Name)
			if err != nil {
				return err
			}
		} else if remoteVM.Status == v1.VMStatusRunning && !worker.vmm.Exists(v1.VM{UID: onDiskName.UID}) {
			// On-disk VM exist on the controller,
			// but we don't know about it, so
			// mark it as failed
			remoteVM.Status = v1.VMStatusFailed
			remoteVM.StatusMessage = "Worker lost track of VM"
			_, err := worker.client.VMs().Update(ctx, remoteVM)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (worker *Worker) updateRemoteStatus(
	ctx context.Context,
	vm v1.VM,
	status v1.VMStatus,
	message string,
	args ...any,
) {
	vm.Status = status
	vm.StatusMessage = fmt.Sprintf(message, args...)

	_, err := worker.client.VMs().Update(ctx, vm)
	if err != nil {
		worker.logger.Warnf("failed to update remote VM: %v", err)
	}
}

func (worker *Worker) grpcMetadata() metadata.MD {
	return metadata.Join(
		worker.client.GPRCMetadata(),
		metadata.Pairs(rpc.MetadataWorkerNameKey, worker.name),
	)
}

func (worker *Worker) requestVMSyncing() {
	select {
	case worker.syncRequested <- true:
		worker.logger.Debugf("Successfully requested syncing")
	default:
		worker.logger.Debugf("There's already a syncing request in the queue, skipping")
	}
}

func localVMState(vm *vmpkg.VM) *vmpkg.State {
	if vm != nil {
		result := vm.State()

		return &result
	}

	return nil
}

func localVMStateDescription(vm *vmpkg.VM) string {
	if vm != nil {
		return fmt.Sprintf("in state %q", vm.State())
	}

	return "(non-existent)"
}

func remoteVMState(vm *v1.VM) *v1.VMStatus {
	if vm != nil {
		result := vm.Status

		return &result
	}

	return nil
}

func remoteVMStateDescription(vm *v1.VM) string {
	if vm != nil {
		return fmt.Sprintf("in state %q", vm.Status)
	}

	return "(non-existent)"
}
