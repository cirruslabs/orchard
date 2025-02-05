package v1

import (
	"time"
)

// Meta is a common set of fields that apply to all resources managed by the Controller.
type Meta struct {
	// Name is a human-readable resource identifier populated by the Worker or Client.
	//
	// There can't be multiple resources with the same Name in the DB at any given time.
	Name string `json:"name,omitempty"`

	// CreatedAt is a useful field for scheduler prioritization.
	//
	// It is populated by the Controller with the current time
	// when receiving a POST request.
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

type VM struct {
	Image           string          `json:"image,omitempty"`
	ImagePullPolicy ImagePullPolicy `json:"imagePullPolicy,omitempty"`
	CPU             uint64          `json:"cpu,omitempty"`
	Memory          uint64          `json:"memory,omitempty"`
	NetSoftnet      bool            `json:"net-softnet,omitempty"`
	NetBridged      string          `json:"net-bridged,omitempty"`
	Headless        bool            `json:"headless,omitempty"`

	// Status field is used to track the lifecycle of the VM associated with this resource.
	Status        VMStatus `json:"status,omitempty"`
	StatusMessage string   `json:"status_message,omitempty"`

	// Worker field is set by the Controller to assign this VM to a specific Worker.
	Worker string `json:"worker,omitempty"`

	// AssignedCPU is set by the Controller when the VM is scheduled.
	//
	// It's set to CPU when CPU non-zero, otherwise the value is taken from
	// Worker's DefaultCPU field. If Worker's DefaultCPU field is zero, it defaults
	// to 4.
	AssignedCPU uint64 `json:"assignedCPU,omitempty"`
	// AssignedMemory is set by the Controller
	//
	// It's set to Memory when Memory non-zero, otherwise the value is taken from
	// Worker's DefaultCPU field. If Worker's DefaultCPU field is zero, it defaults
	// to 8192.
	AssignedMemory uint64 `json:"assignedMemory,omitempty"`

	Username      string    `json:"username,omitempty"`
	Password      string    `json:"password,omitempty"`
	StartupScript *VMScript `json:"startup_script,omitempty"`

	RestartPolicy RestartPolicy `json:"restart_policy,omitempty"`
	RestartedAt   time.Time     `json:"restarted_at,omitempty"`
	RestartCount  uint64        `json:"restart_count,omitempty"`

	// UID is a useful field for avoiding data races within a single Name.
	//
	// It is populated by the Controller when receiving a POST request.
	UID string `json:"uid,omitempty"`

	// Resources required by this VM.
	Resources Resources `json:"resources,omitempty"`

	// HostDir is a list of host directories to be mounted to the VM.
	HostDirs []HostDir `json:"hostDirs,omitempty"`

	// ImageFQN is a fully qualified name of the Image that it is populated
	// by the worker using "tart fqn" command after it had pulled the image.
	ImageFQN string `json:"image_fqn,omitempty"`

	ScheduledAt time.Time `json:"scheduled_at,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`

	Meta
}

type Event struct {
	Kind      EventKind `json:"kind,omitempty"`
	Timestamp int64     `json:"timestamp,omitempty"`
	Payload   string    `json:"payload,omitempty"`
}

type EventKind string

const (
	EventKindLogLine EventKind = "log_line"
)

type VMScript struct {
	ScriptContent string            `json:"script_content,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
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

type ControllerCapability string

const (
	ControllerCapabilityRPCV1 ControllerCapability = "rpc-v1"
	ControllerCapabilityRPCV2 ControllerCapability = "rpc-v2"
)

type ControllerCapabilities []ControllerCapability

func (controllerCapabilities ControllerCapabilities) Has(capability ControllerCapability) bool {
	for _, controllerCapability := range controllerCapabilities {
		if controllerCapability == capability {
			return true
		}
	}

	return false
}

type ControllerInfo struct {
	Version      string                 `json:"version,omitempty"`
	Commit       string                 `json:"commit,omitempty"`
	Capabilities ControllerCapabilities `json:"capabilities,omitempty"`
}
