package scheduler

import (
	"cmp"
	"context"
	"errors"
	"github.com/cirruslabs/orchard/internal/controller/lifecycle"
	"github.com/cirruslabs/orchard/internal/controller/notifier"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/internal/opentelemetry"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/cirruslabs/orchard/rpc"
	mapset "github.com/deckarep/golang-set/v2"
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
	ErrVMSchedulingSkipped     = errors.New("scheduling skipped for VM")
	ErrWorkerSchedulingSkipped = errors.New("scheduling skipped for worker")
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
	prometheusMetrics    bool

	schedulingTimeHistogram metric.Float64Histogram
}

func NewScheduler(
	store storepkg.Store,
	notifier *notifier.Notifier,
	workerOfflineTimeout time.Duration,
	prometheusMetrics bool,
	logger *zap.SugaredLogger,
) (*Scheduler, error) {
	scheduler := &Scheduler{
		store:                store,
		notifier:             notifier,
		workerOfflineTimeout: workerOfflineTimeout,
		logger:               logger,
		schedulingRequested:  make(chan bool, 1),
		prometheusMetrics:    prometheusMetrics,
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

		healthCheckingLoopIterationStart := time.Now()
		if err := scheduler.healthCheckingLoopIteration(); err != nil {
			scheduler.logger.Errorf("Failed to health-check VMs: %v", err)
		}
		healthCheckingLoopIterationEnd := time.Now()

		schedulingLoopIterationStart := time.Now()
		err := scheduler.schedulingLoopIteration()
		schedulingLoopIterationEnd := time.Now()

		scheduler.logger.Debugf("Health checking loop iteration took %v, "+
			"scheduling loop iteration took %v",
			healthCheckingLoopIterationEnd.Sub(healthCheckingLoopIterationStart),
			schedulingLoopIterationEnd.Sub(schedulingLoopIterationStart))

		if err != nil {
			scheduler.logger.Errorf("Failed to schedule VMs: %v", err)
		}

		if scheduler.prometheusMetrics {
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

//nolint:gocognit,gocyclo // this logic could be seen as even more complex if split into multiple functions
func (scheduler *Scheduler) schedulingLoopIteration() error {
	affectedWorkers := mapset.NewSet[string]()

	// Scheduler consistency model is based on the following:
	//
	// EXCLUSIVENESS:
	//
	// Only one scheduler might operate in a cluster at any given time.
	//
	// Currently, this is achieved automatically since we run in-process
	// BadgerDB that runs in the Orchard Controller, and Orchard Controller
	// in turn runs a single scheduler.
	//
	// In the future, we might support etcd, and in that case leader
	// election can be implemented to ensure this property, thanks to
	// etcd leases[1].
	//
	// [1]: https://medium.com/@ahadrana/understanding-etcd3-8784c4f61755
	//
	// OVERESTIMATION OF USED RESOURCES:
	//
	// Scheduler acts opportunistically on a lagging view of resource
	// usage in the cluster.
	//
	// This means that we won't assign more than what a worker can handle,
	// but we might skip the worker from the consideration, even through
	// in reality it can already handle the VM we're scheduling, and assign
	// the VM to a next worker, thus slightly violating the scheduler
	// profile at times.
	//
	// LAGGING SCHEDULER PROFILE:
	//
	// In case the scheduler profile is changed amidst the scheduling loop
	// iteration, we'll act on a previously set scheduler profile.
	//
	// It feels that this is totally fine, assuming that (1) the scheduler
	// profile is not something that's changed frequently and that (2) at
	// the same time when the scheduler profile is changed, a user won't
	// schedule a bunch of VMs in the hope that they'll be serviced using
	// that new scheduling profile.

	var vms []v1.VM
	var workers []v1.Worker
	var schedulerProfile v1.SchedulerProfile

	if err := scheduler.store.View(func(txn storepkg.Transaction) error {
		var err error

		vms, err = txn.ListVMs()
		if err != nil {
			return err
		}

		workers, err = txn.ListWorkers()
		if err != nil {
			return err
		}

		clusterSettings, err := txn.GetClusterSettings()
		if err != nil {
			return err
		}
		schedulerProfile = clusterSettings.SchedulerProfile

		return nil
	}); err != nil {
		return err
	}

	unscheduledVMs, workerInfos := ProcessVMs(vms)

NextVM:
	for _, unscheduledVM := range unscheduledVMs {
		// Order workers depending on the scheduler profile and
		// our updated lagging resource usage for each worker
		switch schedulerProfile {
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

		// Iterate through sorted workers and find a worker that can run this VM
	NextWorker:
		for _, worker := range workers {
			resourcesUsed := workerInfos.Get(worker.Name).ResourcesUsed
			resourcesRemaining := worker.Resources.Subtracted(resourcesUsed)

			if worker.Offline(scheduler.workerOfflineTimeout) ||
				worker.SchedulingPaused ||
				!resourcesRemaining.CanFit(unscheduledVM.Resources) ||
				!worker.Labels.Contains(unscheduledVM.Labels) {
				continue NextWorker
			}

			err := scheduler.store.Update(func(txn storepkg.Transaction) error {
				currentUnscheduledVM, err := txn.GetVM(unscheduledVM.Name)
				if err != nil {
					if errors.Is(err, storepkg.ErrNotFound) {
						// The unscheduled VM ceased to exist,
						// so nothing to schedule
						return ErrVMSchedulingSkipped
					}

					return err
				}

				if currentUnscheduledVM.UID != unscheduledVM.UID {
					// The unscheduled VM had changed, so we'll re-evaluate a new
					// version of it in the next scheduling loop iteration
					return ErrVMSchedulingSkipped
				}

				if currentUnscheduledVM.Status != v1.VMStatusPending ||
					currentUnscheduledVM.Worker != "" {
					// Unscheduled VM is not unscheduled anymore,
					// so there's nothing to do
					return ErrVMSchedulingSkipped
				}

				currentWorker, err := txn.GetWorker(worker.Name)
				if err != nil {
					if errors.Is(err, storepkg.ErrNotFound) {
						// The worker that we were planning to schedule
						// this VM on has ceased to exist, so move on
						return ErrWorkerSchedulingSkipped
					}

					return err
				}

				if currentWorker.Offline(scheduler.workerOfflineTimeout) ||
					currentWorker.SchedulingPaused {
					return ErrWorkerSchedulingSkipped
				}

				if currentWorker.MachineID != worker.MachineID ||
					!currentWorker.Resources.Equal(worker.Resources) {
					// Worker has changed
					return ErrWorkerSchedulingSkipped
				}

				unscheduledVM.Worker = worker.Name
				unscheduledVM.ScheduledAt = time.Now()

				// Fill out the actual CPU allocation
				if unscheduledVM.CPU == 0 {
					// Provide defaults for VMs with implicit CPU specification
					if worker.DefaultCPU != 0 {
						unscheduledVM.AssignedCPU = worker.DefaultCPU
					} else {
						unscheduledVM.AssignedCPU = 4
					}
				} else {
					unscheduledVM.AssignedCPU = unscheduledVM.CPU
				}

				// Fill out the actual memory allocation
				if unscheduledVM.Memory == 0 {
					// Provide defaults for VMs with implicit memory specification
					if worker.DefaultMemory != 0 {
						unscheduledVM.AssignedMemory = worker.DefaultMemory
					} else {
						unscheduledVM.AssignedMemory = 8192
					}
				} else {
					unscheduledVM.AssignedMemory = unscheduledVM.Memory
				}

				if err := txn.SetVM(unscheduledVM); err != nil {
					return err
				}

				return nil
			})
			if err != nil {
				if errors.Is(err, ErrVMSchedulingSkipped) {
					continue NextVM
				}

				if errors.Is(err, ErrWorkerSchedulingSkipped) {
					continue NextWorker
				}

				return err
			}

			// Update lagging resource usage
			workerInfos.AddVM(worker.Name, unscheduledVM.Resources)

			// Ping the worker afterward for faster VM execution
			affectedWorkers.Add(worker.Name)

			// Update metrics
			scheduler.schedulingTimeHistogram.Record(context.Background(),
				time.Since(unscheduledVM.CreatedAt).Seconds())

			break
		}
	}

	for affectedWorker := range affectedWorkers.Iter() {
		// It's fine to not treat the error as fatal here,
		// since the worker will sync the VMs on the next
		// scheduling iteration
		notifyContext, notifyContextCancel := context.WithTimeout(context.Background(), time.Second)
		if err := scheduler.notifier.Notify(notifyContext, affectedWorker, &rpc.WatchInstruction{
			Action: &rpc.WatchInstruction_SyncVmsAction{},
		}); err != nil {
			scheduler.logger.Errorf("Failed to reactively sync VMs on worker %s: %v", affectedWorker, err)
		}
		notifyContextCancel()
	}

	return nil
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
	// Get a lagging view of VMs
	var vms []v1.VM

	if err := scheduler.store.View(func(txn storepkg.Transaction) error {
		var err error

		vms, err = txn.ListVMs()
		if err != nil {
			return err
		}

		// Update metrics
		if scheduler.prometheusMetrics {
			workers, err := txn.ListWorkers()
			if err != nil {
				return err
			}

			scheduler.reportStats(workers, vms)
		}

		return nil
	}); err != nil {
		return err
	}

	// Process each VM in a lagging list of VMs in an individual
	// transaction, re-checking that the VM still exists
	// and it is still scheduled
	for _, vm := range vms {
		if vm.Worker == "" {
			// Not a scheduled VM
			//
			// We'll re-check this below, but this allows us
			// to avoid wasting cycles opening a transaction
			// for nothing.
			continue
		}

		if err := scheduler.store.Update(func(txn storepkg.Transaction) error {
			currentVM, err := txn.GetVM(vm.Name)
			if err != nil {
				if errors.Is(err, storepkg.ErrNotFound) {
					// VM ceased to exist, nothing to do
					return nil
				}

				return err
			}

			if currentVM.Worker == "" {
				// Not a scheduled VM, nothing to do
				return nil
			}

			return scheduler.healthCheckVM(txn, *currentVM)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (scheduler *Scheduler) healthCheckVM(txn storepkg.Transaction, vm v1.VM) error {
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
		vm.AssignedCPU = 0
		vm.AssignedMemory = 0
		vm.RestartedAt = time.Now()
		vm.RestartCount++
		vm.ScheduledAt = time.Time{}
		vm.StartedAt = time.Time{}

		return txn.SetVM(vm)
	}

	worker, err := txn.GetWorker(vm.Worker)
	if err != nil {
		if errors.Is(err, storepkg.ErrNotFound) {
			vm.Status = v1.VMStatusFailed
			vm.StatusMessage = "VM is assigned to a worker that " +
				"doesn't exist anymore"

			return txn.SetVM(vm)
		}

		return err
	}

	if worker.Offline(scheduler.workerOfflineTimeout) && !vm.TerminalState() {
		vm.Status = v1.VMStatusFailed
		vm.StatusMessage = "VM is assigned to a worker that " +
			"lost connection with the controller"

		return txn.SetVM(vm)
	}

	return nil
}
