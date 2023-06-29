package worker

import (
	"context"
	"errors"
	"fmt"
	"github.com/avast/retry-go/v4"
	"github.com/cirruslabs/orchard/internal/worker/iokitregistry"
	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/cirruslabs/orchard/internal/worker/tart"
	"github.com/cirruslabs/orchard/internal/worker/vmmanager"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
	"os"
	"time"
)

const pollInterval = 5 * time.Second

var ErrPollFailed = errors.New("failed to poll controller")

type Worker struct {
	name          string
	syncRequested chan bool
	vmm           *vmmanager.VMManager
	client        *client.Client
	pollTicker    *time.Ticker
	resources     v1.Resources
	logger        *zap.SugaredLogger
}

func New(client *client.Client, opts ...Option) (*Worker, error) {
	worker := &Worker{
		client:        client,
		pollTicker:    time.NewTicker(pollInterval),
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

		select {
		case <-worker.pollTicker.C:
			// continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (worker *Worker) Close() error {
	var result error
	for _, vm := range worker.vmm.List() {
		err := vm.Stop()
		if err != nil {
			result = multierror.Append(result, err)
		}
	}
	for _, vm := range worker.vmm.List() {
		err := vm.Delete()
		if err != nil {
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

		return nil
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
		case <-worker.pollTicker.C:
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
	remoteVMsIndex := map[ondiskname.OnDiskName]v1.VM{}
	for _, remoteVM := range remoteVMs {
		remoteVMsIndex[ondiskname.NewFromResource(remoteVM)] = remoteVM
	}

	worker.logger.Infof("syncing %d local VMs against %d remote VMs...",
		len(remoteVMsIndex), worker.vmm.Len())

	for _, vm := range worker.vmm.List() {
		remoteVM, ok := remoteVMsIndex[vm.OnDiskName()]
		if !ok {
			// Remote VM was deleted, delete local VM
			//
			// Note: this check needs to run for each VM
			// before we attempt to create any VMs below.
			if err := worker.deleteVM(vm); err != nil {
				return err
			}
		} else if remoteVM.Status != v1.VMStatusFailed && vm.Err() != nil {
			// Local VM has failed, update remote VM
			remoteVM.Status = v1.VMStatusFailed
			remoteVM.StatusMessage = vm.Err().Error()
			if _, err := worker.client.VMs().Update(ctx, remoteVM); err != nil {
				return err
			}
		}
	}

	for _, vmResource := range remoteVMsIndex {
		odn := ondiskname.NewFromResource(vmResource)

		if vmResource.Status == v1.VMStatusPending && !worker.vmm.Exists(odn) {
			// Remote VM was created, create local VM
			if err := worker.createVM(ctx, odn, vmResource); err != nil {
				return err
			}

			vmResource.Status = v1.VMStatusRunning
			if _, err := worker.client.VMs().Update(ctx, vmResource); err != nil {
				return err
			}
		}
	}

	return nil
}

func (worker *Worker) syncOnDiskVMs(ctx context.Context) error {
	remoteVMs, err := worker.client.VMs().FindForWorker(ctx, worker.name)
	if err != nil {
		return err
	}
	remoteVMsIndex := map[ondiskname.OnDiskName]v1.VM{}
	for _, remoteVM := range remoteVMs {
		remoteVMsIndex[ondiskname.NewFromResource(remoteVM)] = remoteVM
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

		remoteVM, ok := remoteVMsIndex[onDiskName]
		if !ok {
			// On-disk VM doesn't exist on the controller, delete it
			_, _, err := tart.Tart(ctx, worker.logger, "delete", vmInfo.Name)
			if err != nil {
				return err
			}
		} else if remoteVM.Status == v1.VMStatusRunning && !worker.vmm.Exists(onDiskName) {
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

func (worker *Worker) deleteVM(vm *vmmanager.VM) error {
	if err := vm.Stop(); err != nil {
		return err
	}

	if err := vm.Delete(); err != nil {
		return err
	}

	worker.vmm.Delete(vm.OnDiskName())

	return nil
}

func (worker *Worker) createVM(ctx context.Context, odn ondiskname.OnDiskName, vmResource v1.VM) error {
	eventStreamer := worker.client.VMs().StreamEvents(vmResource.Name)

	vm, err := vmmanager.NewVM(ctx, vmResource, eventStreamer, worker.logger)
	if err != nil {
		return err
	}

	worker.vmm.Put(odn, vm)

	return nil
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
