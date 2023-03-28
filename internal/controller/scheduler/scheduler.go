package scheduler

import (
	"context"
	"github.com/cirruslabs/orchard/internal/controller/notifier"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"go.uber.org/zap"
	"sort"
	"time"
)

const schedulerInterval = 5 * time.Second

type Scheduler struct {
	store               storepkg.Store
	notifier            *notifier.Notifier
	logger              *zap.SugaredLogger
	schedulingRequested chan bool
}

func NewScheduler(store storepkg.Store, notifier *notifier.Notifier, logger *zap.SugaredLogger) *Scheduler {
	return &Scheduler{
		store:               store,
		notifier:            notifier,
		logger:              logger,
		schedulingRequested: make(chan bool, 1),
	}
}

func (scheduler *Scheduler) Run() {
	for {
		// wait either the scheduling interval or a request to schedule
		select {
		case <-scheduler.schedulingRequested:
		case <-time.After(schedulerInterval):
		}
		if err := scheduler.schedulingLoopIteration(); err != nil {
			scheduler.logger.Errorf("Failed to schedule VMs: %v", err)
		}
	}
}

func (scheduler *Scheduler) RequestScheduling() {
	select {
	case scheduler.schedulingRequested <- true:
		scheduler.logger.Debugf("Successfully requested scheduling")
	default:
		scheduler.logger.Debugf("There's already a scheduling request in the queue, skipping")
	}
}

func (scheduler *Scheduler) schedulingLoopIteration() error {
	affectedWorkers := map[string]bool{}
	err := scheduler.store.Update(func(txn storepkg.Transaction) error {
		vms, err := txn.ListVMs()
		if err != nil {
			return err
		}
		unscheduledVMs, workerToResources := processVMs(vms)

		workers, err := txn.ListWorkers()
		if err != nil {
			return err
		}

		for _, unscheduledVM := range unscheduledVMs {
			// Find a worker that can run this VM
			for _, worker := range workers {
				resourcesUsed := workerToResources.Get(worker.Name)
				resourcesRemaining := worker.Resources.Subtracted(resourcesUsed)

				if resourcesRemaining.CanFit(unscheduledVM.Resources) {
					unscheduledVM.Worker = worker.Name

					if err := txn.SetVM(unscheduledVM); err != nil {
						return err
					}
					affectedWorkers[worker.Name] = true

					workerToResources.Add(worker.Name, unscheduledVM.Resources)
				}
			}
		}

		return nil
	})
	syncVMsInstruction := rpc.WatchInstruction{
		Action: &rpc.WatchInstruction_SyncVmsAction{},
	}
	for workerToPoke := range affectedWorkers {
		// it's fine to ignore the error here, since the worker will sync the VMs on the next cycle
		notifyErr := scheduler.notifier.Notify(context.Background(), workerToPoke, &syncVMsInstruction)
		if notifyErr != nil {
			scheduler.logger.Errorf("Failed to reactively sync VMs on worker %s: %v", workerToPoke, notifyErr)
		}
	}
	return err
}

func processVMs(vms []v1.VM) ([]v1.VM, WorkerToResources) {
	var unscheduledVMs []v1.VM
	workerToResources := make(WorkerToResources)

	for _, vm := range vms {
		if vm.Worker == "" {
			unscheduledVMs = append(unscheduledVMs, vm)
		} else if !vm.TerminalState() {
			workerToResources.Add(vm.Worker, vm.Resources)
		}
	}

	// Sort unscheduled VMs by the date of creation
	sort.Slice(unscheduledVMs, func(i, j int) bool {
		return unscheduledVMs[i].CreatedAt.Before(unscheduledVMs[j].CreatedAt)
	})

	return unscheduledVMs, workerToResources
}
