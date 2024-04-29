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

	Meta
}

func (worker Worker) Offline(workerOfflineTimeout time.Duration) bool {
	return time.Since(worker.LastSeen) > workerOfflineTimeout
}
