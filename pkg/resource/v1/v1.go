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
	Image           string          `json:"image"`
	ImagePullPolicy ImagePullPolicy `json:"imagePullPolicy"`
	CPU             uint64          `json:"cpu"`
	Memory          uint64          `json:"memory"`
	NetSoftnet      bool            `json:"net-softnet"`
	NetBridged      string          `json:"net-bridged"`
	Headless        bool            `json:"headless"`

	// Status field is used to track the lifecycle of the VM associated with this resource.
	Status        VMStatus `json:"status"`
	StatusMessage string   `json:"status_message"`

	// Worker field is set by the Controller to assign this VM to a specific Worker.
	Worker string `json:"worker"`

	Username      string    `json:"username"`
	Password      string    `json:"password"`
	StartupScript *VMScript `json:"startup_script"`

	RestartPolicy RestartPolicy `json:"restart_policy"`
	RestartedAt   time.Time     `json:"restarted_at"`
	RestartCount  uint64        `json:"restart_count"`

	// UID is a useful field for avoiding data races within a single Name.
	//
	// It is populated by the Controller when receiving a POST request.
	UID string `json:"uid"`

	// Resources required by this VM.
	Resources Resources `json:"resources"`

	// HostDir is a list of host directories to be mounted to the VM.
	HostDirs []HostDir `json:"hostDirs"`

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
	return vm.Status == VMStatusFailed
}

type VMStatus string

func (vmStatus VMStatus) String() string {
	return string(vmStatus)
}

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

type ControllerInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}
