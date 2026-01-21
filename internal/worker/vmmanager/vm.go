package vmmanager

import (
	"context"

	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
)

type VM interface {
	Resource() v1.VM
	SetResource(vmResource v1.VM)
	OnDiskName() ondiskname.OnDiskName
	ImageFQN() *string
	Status() v1.VMStatus
	StatusMessage() string
	Err() error
	Conditions() []v1.Condition

	Start(eventStreamer *client.EventStreamer)
	Suspend() <-chan error
	IP(ctx context.Context) (string, error)
	Stop() <-chan error
	Delete() error
}
