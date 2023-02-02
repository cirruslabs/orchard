package worker

import (
	"context"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/worker/vmmanager"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"go.uber.org/zap"
	"os"
	"time"
)

const pollInterval = 5 * time.Second

var ErrPollFailed = errors.New("failed to poll controller")

type Worker struct {
	dataDirPath string
	name        string
	uid         string
	vmm         *vmmanager.VMManager
	client      *client.Client
	logger      *zap.SugaredLogger
}

func New(opts ...Option) (*Worker, error) {
	worker := &Worker{
		vmm: vmmanager.New(),
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
	if worker.logger == nil {
		worker.logger = zap.NewNop().Sugar()
	}

	// Instantiate worker
	client, err := client.New()
	if err != nil {
		return nil, err
	}
	worker.client = client

	return worker, nil
}

func (worker *Worker) Run(ctx context.Context) error {
	tickCh := time.NewTicker(pollInterval)

	for {
		if err := worker.registerWorker(ctx); err != nil {
			worker.logger.Warnf("failed to register worker: %v", err)

			select {
			case <-tickCh.C:
				// continue
			case <-ctx.Done():
				return ctx.Err()
			}

			continue
		}

		for {
			if err := worker.updateWorker(ctx); err != nil {
				worker.logger.Errorf("failed to update worker resource: %v", err)

				select {
				case <-tickCh.C:
					// continue
				case <-ctx.Done():
					return ctx.Err()
				}

				break
			}

			if err := worker.syncVMs(ctx); err != nil {
				worker.logger.Warnf("failed to sync VMs: %v", err)

				select {
				case <-tickCh.C:
					// continue
				case <-ctx.Done():
					return ctx.Err()
				}

				break
			}

			select {
			case <-tickCh.C:
				// continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func (worker *Worker) registerWorker(ctx context.Context) error {
	workerResource := &v1.Worker{
		Meta: v1.Meta{
			Name: worker.name,
		},
		LastSeen: time.Now(),
	}

	workerResource, err := worker.client.Workers().Create(ctx, workerResource)
	if err != nil {
		return err
	}

	worker.uid = workerResource.UID

	worker.logger.Infof("registered worker %s with UID %s", worker.name, workerResource.UID)

	return nil
}

func (worker *Worker) updateWorker(ctx context.Context) error {
	workerResource, err := worker.client.Workers().Get(ctx, worker.name)
	if err != nil {
		return fmt.Errorf("%w: failed to retrieve worker from the API: %v", ErrPollFailed, err)
	}

	if workerResource.UID != worker.uid {
		return fmt.Errorf("%w: our UID is %s, controller's UID is %s",
			ErrPollFailed, worker.uid, workerResource.UID)
	}

	worker.logger.Debugf("got worker from the API")

	workerResource.LastSeen = time.Now()

	if err := worker.client.Workers().Update(ctx, workerResource); err != nil {
		return fmt.Errorf("%w: failed to update worker in the API: %v", ErrPollFailed, err)
	}

	worker.logger.Debugf("updated worker in the API")

	return nil
}

func (worker *Worker) syncVMs(ctx context.Context) error {
	vms, err := worker.client.VMs().List(ctx)
	if err != nil {
		return err
	}

	worker.logger.Infof("syncing %d VMs...", len(vms))

	for _, vmResource := range vms {
		vmResource := vmResource

		if vmResource.Worker != worker.name {
			continue
		}

		if !vmResource.DeletedAt.IsZero() {
			if err := worker.deleteVM(ctx, vmResource); err != nil {
				return err
			}
		} else if !worker.vmm.Exists(&vmResource) {
			if err := worker.createVM(ctx, vmResource); err != nil {
				return err
			}
		}
	}

	return nil
}

func (worker *Worker) deleteVM(ctx context.Context, vmResource v1.VM) error {
	worker.logger.Debugf("deleting VM %s (%s)", vmResource.Name, vmResource.UID)

	// Delete VM locally, report to the controller
	if worker.vmm.Exists(&vmResource) {
		if err := worker.vmm.Delete(&vmResource); err != nil {
			return err
		}
	}

	if err := worker.client.VMs().Delete(ctx, vmResource.Name, true); err != nil {
		return fmt.Errorf("%w: failed to delete VM %s (%s) from the API: %v",
			ErrPollFailed, vmResource.Name, vmResource.UID, err)
	}

	worker.logger.Infof("deleted VM %s (%s)", vmResource.Name, vmResource.UID)

	return nil
}

func (worker *Worker) createVM(ctx context.Context, vmResource v1.VM) error {
	worker.logger.Debugf("creating VM %s (%s)", vmResource.Name, vmResource.UID)

	// Create or update VM locally, report to controller
	_, err := worker.vmm.Create(&vmResource)
	if err != nil {
		return err
	}

	vmResource.Status = v1.VMStatusRunning

	if err := worker.client.VMs().Update(ctx, &vmResource); err != nil {
		return fmt.Errorf("%w: failed to update VM %s (%s) in the API: %v",
			ErrPollFailed, vmResource.Name, vmResource.UID, err)
	}

	worker.logger.Infof("spawned VM %s (%s)", vmResource.Name, vmResource.UID)

	return nil
}
