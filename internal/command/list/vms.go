package list

import (
	"fmt"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
)

func newListVMsCommand() *cobra.Command {
	command := &cobra.Command{
		Use:  "vms",
		RunE: runListVMs,
	}

	return command
}

func runListVMs(cmd *cobra.Command, args []string) error {
	client, err := client.New()
	if err != nil {
		return err
	}

	vms, err := client.VMs().List(cmd.Context())
	if err != nil {
		return err
	}

	if quiet {
		for _, vm := range vms {
			fmt.Println(vm.Name)
		}

		return nil
	}

	table := uitable.New()

	table.AddRow("Name", "Image", "Status")

	for _, vm := range vms {
		table.AddRow(vm.Name, vm.Image, vm.Status)
	}

	fmt.Println(table)

	return nil
}
