package worker

import (
	"testing"

	"github.com/cirruslabs/orchard/internal/worker/ondiskname"
	"github.com/cirruslabs/orchard/internal/worker/vmmanager"
	"github.com/cirruslabs/orchard/internal/worker/vmmanager/tart"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

func TestSortNonExistentAndFailedFirst(t *testing.T) {
	newVMTuple := func(name string, vmResource *v1.VM) lo.Tuple3[ondiskname.OnDiskName, *v1.VM, vmmanager.VM] {
		return lo.T3[ondiskname.OnDiskName, *v1.VM, vmmanager.VM](
			ondiskname.New(name, name, 0),
			vmResource,
			&tart.VM{},
		)
	}

	target := []lo.Tuple3[ondiskname.OnDiskName, *v1.VM, vmmanager.VM]{
		newVMTuple("test1", &v1.VM{Status: v1.VMStatusFailed}),
		newVMTuple("test2", &v1.VM{Status: v1.VMStatusPending}),
		newVMTuple("test3", &v1.VM{Status: v1.VMStatusRunning}),
		newVMTuple("test5", nil),
		newVMTuple("test4", &v1.VM{Status: v1.VMStatusFailed}),
	}

	sortNonExistentAndFailedFirst(target)

	expected := []lo.Tuple3[ondiskname.OnDiskName, *v1.VM, vmmanager.VM]{
		newVMTuple("test5", nil),
		newVMTuple("test1", &v1.VM{Status: v1.VMStatusFailed}),
		newVMTuple("test4", &v1.VM{Status: v1.VMStatusFailed}),
		newVMTuple("test2", &v1.VM{Status: v1.VMStatusPending}),
		newVMTuple("test3", &v1.VM{Status: v1.VMStatusRunning}),
	}

	require.Equal(t, expected, target)
}
