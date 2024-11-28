package scheduler

import (
	"cmp"
	"context"
	"github.com/cirruslabs/orchard/internal/controller/lifecycle"
	"github.com/cirruslabs/orchard/internal/controller/notifier"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/opentelemetry"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
	"slices"
	"sort"
	"time"
)

const (
	schedulerInterval = 5 * time.Second

	schedulerVMRestartDelay = 15 * time.Second
)

var (
	schedulerLoopIterationStat = promauto.NewCounter(prometheus.CounterOpts{
		Name: "orchard_scheduler_loop_iteration_total",
	})
	workersStat = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "orchard_workers",
	}, []string{"worker_name", "status"})
	vmsStat = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "orchard_vms",
	}, []string{"status"})
)

type Scheduler struct {
	store                storepkg.Store
	notifier             *notifier.Notifier
	workerOfflineTimeout time.Duration
	logger               *zap.SugaredLogger
	schedulingRequested  chan bool

	schedulingTimeHistogram metric.Float64Histogram
}

func NewScheduler(
	store storepkg.Store,
	notifier *notifier.Notifier,
	workerOfflineTimeout time.Duration,
	logger *zap.SugaredLogger,
) (*Scheduler, error) {
	scheduler := &Scheduler{
		store:                store,
		notifier:             notifier,
		workerOfflineTimeout: workerOfflineTimeout,
		logger:               logger,
		schedulingRequested:  make(chan bool, 1),
	}

	// Metrics
	var err error

	scheduler.schedulingTimeHistogram, err = opentelemetry.DefaultMeter.
		Float64Histogram("org.cirruslabs.orchard.controller.scheduling_time")
	if err != nil {
		return nil, err
	}

	return scheduler, nil
}

func (scheduler *Scheduler) Run() {
	for {
		// wait either the scheduling interval or a request to schedule
		select {
		case <-scheduler.schedulingRequested:
		case <-time.After(schedulerInterval):
		}

		if err := scheduler.healthCheckingLoopIteration(); err != nil {
			scheduler.logger.Errorf("Failed to health-check VMs: %v", err)
		}
		if err := scheduler.schedulingLoopIteration(); err != nil {
			scheduler.logger.Errorf("Failed to schedule VMs: %v", err)
		} else {
			schedulerLoopIterationStat.Inc()
		}
	}
}

func (scheduler *Scheduler) reportStats(workers []v1.Worker, vms []v1.VM) {
	for _, worker := range workers {
		if worker.Offline(scheduler.workerOfflineTimeout) {
			workersStat.With(map[string]string{"worker_name": worker.Name, "status": "online"}).Set(0)
			workersStat.With(map[string]string{"worker_name": worker.Name, "status": "offline"}).Set(1)
		} else {
			workersStat.With(map[string]string{"worker_name": worker.Name, "status": "online"}).Set(1)
			workersStat.With(map[string]string{"worker_name": worker.Name, "status": "offline"}).Set(0)
		}
	}
	for _, vm := range vms {
		vmsStat.With(map[string]string{"status": string(vm.Status)}).Inc()
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
		unscheduledVMs, workerInfos := ProcessVMs(vms)

		workers, err := txn.ListWorkers()
		if err != nil {
			return err
		}

		// Retrieve cluster settings to figure out which scheduler profile to use
		clusterSettings, err := txn.GetClusterSettings()
		if err != nil {
			return err
		}

		for _, unscheduledVM := range unscheduledVMs {
			// Order workers depending on the scheduler profile
			switch clusterSettings.SchedulerProfile {
			case v1.SchedulerProfileDistributeLoad:
				slices.SortFunc(workers, func(a, b v1.Worker) int {
					// Sort by the number of running VMs, ascending order
					return cmp.Compare(workerInfos[a.Name].NumRunningVMs,
						workerInfos[b.Name].NumRunningVMs)
				})
			case v1.SchedulerProfileOptimizeUtilization:
				fallthrough
			default:
				slices.SortFunc(workers, func(a, b v1.Worker) int {
					// Sort by the number of running VMs, descending order
					return cmp.Compare(workerInfos[b.Name].NumRunningVMs,
						workerInfos[a.Name].NumRunningVMs)
				})
			}

			// Find a worker that can run this VM
			for _, worker := range workers {
				resourcesUsed := workerInfos.Get(worker.Name).ResourcesUsed
				resourcesRemaining := worker.Resources.Subtracted(resourcesUsed)

				if resourcesRemaining.CanFit(unscheduledVM.Resources) &&
					!worker.Offline(scheduler.workerOfflineTimeout) &&
					!worker.SchedulingPaused {
					// Metrics
					scheduler.schedulingTimeHistogram.Record(context.Background(),
						time.Since(unscheduledVM.CreatedAt).Seconds())

					unscheduledVM.Worker = worker.Name
					unscheduledVM.ScheduledAt = time.Now()

					if err := txn.SetVM(unscheduledVM); err != nil {
						return err
					}
					affectedWorkers[worker.Name] = true

					workerInfos.AddVM(worker.Name, unscheduledVM.Resources)

					break
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

func ProcessVMs(vms []v1.VM) ([]v1.VM, WorkerInfos) {
	var unscheduledVMs []v1.VM
	workerToResources := make(WorkerInfos)

	for _, vm := range vms {
		if vm.Worker == "" {
			unscheduledVMs = append(unscheduledVMs, vm)
		} else if !vm.TerminalState() {
			workerToResources.AddVM(vm.Worker, vm.Resources)
		}
	}

	// Sort unscheduled VMs by the date of creation
	sort.Slice(unscheduledVMs, func(i, j int) bool {
		return unscheduledVMs[i].CreatedAt.Before(unscheduledVMs[j].CreatedAt)
	})

	return unscheduledVMs, workerToResources
}

func (scheduler *Scheduler) healthCheckingLoopIteration() error {
	return scheduler.store.Update(func(txn storepkg.Transaction) error {
		// Retrieve scheduled VMs
		vms, err := txn.ListVMs()
		if err != nil {
			return err
		}

		var scheduledVMs []v1.VM

		for _, vm := range vms {
			if vm.Worker != "" {
				scheduledVMs = append(scheduledVMs, vm)
			}
		}

		// Retrieve and index workers by name
		workers, err := txn.ListWorkers()
		if err != nil {
			return err
		}

		nameToWorker := map[string]v1.Worker{}
		for _, worker := range workers {
			nameToWorker[worker.Name] = worker
		}

		scheduler.reportStats(workers, vms)

		// Process scheduled VMs
		for _, scheduledVM := range scheduledVMs {
			if err := scheduler.healthCheckVM(txn, nameToWorker, scheduledVM); err != nil {
				return err
			}
		}

		return nil
	})
}

func (scheduler *Scheduler) healthCheckVM(txn storepkg.Transaction, nameToWorker map[string]v1.Worker, vm v1.VM) error {
	logger := scheduler.logger.With("vm_name", vm.Name, "vm_uid", vm.UID, "vm_restart_count", vm.RestartCount)

	// Schedule a VM restart if the restart policy mandates it
	needsRestart := vm.RestartPolicy == v1.RestartPolicyOnFailure &&
		vm.Status == v1.VMStatusFailed &&
		time.Since(vm.RestartedAt) > schedulerVMRestartDelay

	if needsRestart {
		logger.Debugf("restarting VM")

		lifecycle.Report(&vm, "VM restarted", scheduler.logger)

		vm.Status = v1.VMStatusPending
		vm.StatusMessage = ""
		vm.Worker = ""
		vm.RestartedAt = time.Now()
		vm.RestartCount++
		vm.ScheduledAt = time.Time{}
		vm.StartedAt = time.Time{}

		return txn.SetVM(vm)
	}

	worker, ok := nameToWorker[vm.Worker]
	if !ok {
		vm.Status = v1.VMStatusFailed
		vm.StatusMessage = "VM is assigned to a worker that " +
			"doesn't exist anymore"

		return txn.SetVM(vm)
	}

	if worker.Offline(scheduler.workerOfflineTimeout) && !vm.TerminalState() {
		vm.Status = v1.VMStatusFailed
		vm.StatusMessage = "VM is assigned to a worker that " +
			"lost connection with the controller"

		return txn.SetVM(vm)
	}

	return nil
}
