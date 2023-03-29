package v1

import "time"

type Worker struct {
	// LastSeen is set by the Worker and is used by the Controller
	// to track unhealthy Workers.
	LastSeen time.Time

	MachineID string

	// Resources available on this Worker.
	Resources Resources `json:"resources"`

	Meta
}

func (worker Worker) Offline() bool {
	return time.Since(worker.LastSeen).Minutes() > 3
}
