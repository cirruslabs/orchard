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
}

type VM struct {
	Image    string `json:"image"`
	CPU      uint64 `json:"cpu"`
	Memory   uint64 `json:"memory"`
	Softnet  bool   `json:"softnet"`
	Headless bool   `json:"headless"`

	// Status field is used to track the lifecycle of the VM associated with this resource.
	Status        VMStatus `json:"status"`
	StatusMessage string   `json:"status_message"`

	// Worker field is set by the Controller to assign this VM to a specific Worker.
	Worker string `json:"worker"`

	Username       string    `json:"username"`
	Password       string    `json:"password"`
	StartupScript  *VMScript `json:"startup_script"`
	ShutdownScript *VMScript `json:"shutdown_script"`

	// UID is a useful field for avoiding data races within a single Name.
	//
	// It is populated by the Controller when receiving a POST request.
	UID string `json:"uid"`

	// Resources required by this VM.
	Resources Resources `json:"resources"`

	Meta
}

type Event struct {
	Kind      EventKind `json:"kind"`
	Timestamp int64     `json:"timestamp"`
	Payload   string    `json:"payload"`
}

type EventKind string

const (
	EventKindLogLine EventKind = "log_line"
)

type VMScript struct {
	ScriptContent string            `json:"script_content"`
	Env           map[string]string `json:"env"`
}

func (vm VM) TerminalState() bool {
	return vm.Status == VMStatusStopped || vm.Status == VMStatusFailed
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

	// VMStatusStopping is set by the Controller to indicate that a VM resource needs to be stopped but not deleted.
	VMStatusStopping VMStatus = "stopping"

	// VMStatusStopped is set by both the Worker to indicate that a particular VM resource has been stopped successfully
	// (either via API or from within a VM via `sudo shutdown -now`).
	VMStatusStopped VMStatus = "stopped"
)
