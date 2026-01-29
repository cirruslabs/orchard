package v1

import "time"

type Worker struct {
	// LastSeen is set by the Worker and is used by the Controller
	// to track unhealthy Workers.
	LastSeen time.Time `json:"last_seen,omitempty"`

	MachineID string `json:"machine_id,omitempty"`

	SchedulingPaused bool `json:"scheduling_paused,omitempty"`

	// Resources available on this Worker.
	Resources Resources `json:"resources,omitempty"`

	// Labels that this Worker supports.
	Labels Labels `json:"labels,omitempty"`

	// DefaultCPU is the amount of CPUs to assign to a VM
	// when it doesn't explicitly request a specific amount.
	DefaultCPU uint64 `json:"defaultCPU,omitempty"`
	// DefaultMemory is the amount of memory to assign to a VM
	// when it doesn't explicitly request a specific amount.
	DefaultMemory uint64 `json:"defaultMemory,omitempty"`

	Meta
}

func (worker Worker) Offline(workerOfflineTimeout time.Duration) bool {
	return time.Since(worker.LastSeen) > workerOfflineTimeout
}

func (worker *Worker) SetVersion(_ uint64) {}

func (worker *Worker) Match(filter Filter) bool {
	return false
}
