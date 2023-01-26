package v1

import (
	"time"
)

// Meta is a common set of fields that apply to all resources managed by the Controller.
type Meta struct {
	// Name is a human-readable resource identifier populated by the Worker or Client.
	//
	// There can't be multiple resources with the same Name in the DB at any given time.
	Name string `json:"name"`

	// CreatedAt is a useful field for scheduler prioritization.
	//
	// It is populated by the Controller with the current time
	// when receiving a POST request.
	CreatedAt time.Time `json:"createdAt"`

	// DeletedAt is a useful field for graceful resource termination.
	//
	// It is populated by the Controller with the current time
	// when receiving a DELETE request.
	DeletedAt time.Time `json:"deletedAt"`

	// UID is a useful field for avoiding data races within a single Name.
	//
	// It is populated by the Controller when receiving a POST request.
	UID string `json:"uid"`

	// Generation is a useful field for avoiding data races within a single UID.
	//
	// It is populated by the controller when receiving POST or PUT requests.
	Generation int64 `json:"generation"`
}

type Worker struct {
	// LastSeen is set by the Worker and is used by the Controller
	// to track unhealthy Workers.
	LastSeen time.Time

	Meta
}

type VM struct {
	Image    string `json:"image"`
	CPU      uint64 `json:"cpu"`
	Memory   uint64 `json:"memory"`
	Softnet  bool   `json:"softnet"`
	Headless bool   `json:"headless"`

	// Status field is used to track the lifecycle of the VM associated with this resource.
	Status VMStatus `json:"status"`

	// Worker field is set by the Controller to assign this VM to a specific Worker.
	Worker string `json:"worker"`

	Meta
}

type VMStatus string

const (
	// VMStatusPending is set by the Controller for all newly-created VM resources.
	VMStatusPending VMStatus = "pending"

	// VMStatusRunning is set by the Worker once it starts running
	// the Virtual Machine associated with this VM resource.
	VMStatusRunning VMStatus = "running"

	// VMStatusFailed is set by both the Controller and the Worker to indicate a failure
	// that prevented the VM resource from reaching the VMStatusRunning state.
	VMStatusFailed VMStatus = "failed"
)
