package create

import (
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/spf13/cobra"
)

var ErrVMFailed = errors.New("failed to create VM")

var image string
var cpu uint64
var memory uint64
var softnet bool
var headless bool
var stringToStringResources map[string]string

func newCreateVMCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "vm NAME",
		Short: "Create a VM",
		RunE:  runCreateVM,
		Args:  cobra.ExactArgs(1),
	}

	command.PersistentFlags().StringVar(&image, "image", "ghcr.io/cirruslabs/macos-ventura-base:latest", "image to use")
	command.PersistentFlags().Uint64Var(&cpu, "cpu", 4, "number of CPUs to use")
	command.PersistentFlags().Uint64Var(&memory, "memory", 8*1024, "megabytes of memory to use")
	command.PersistentFlags().BoolVar(&softnet, "softnet", false, "whether to use Softnet network isolation")
	command.PersistentFlags().BoolVar(&headless, "headless", true, "whether to run without graphics")
	command.PersistentFlags().StringToStringVar(&stringToStringResources, "resources", map[string]string{},
		"resources to request for this VM")

	return command
}

func runCreateVM(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Convert resources
	resources, err := v1.NewResourcesFromStringToString(stringToStringResources)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrVMFailed, err)
	}

	client, err := client.New()
	if err != nil {
		return err
	}

	return client.VMs().Create(cmd.Context(), &v1.VM{
		Meta: v1.Meta{
			Name: name,
		},
		Image:     image,
		CPU:       cpu,
		Memory:    memory,
		Softnet:   softnet,
		Headless:  headless,
		Resources: resources,
	})
}
