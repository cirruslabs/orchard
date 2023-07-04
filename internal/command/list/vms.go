package list

import (
	"fmt"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/dustin/go-humanize"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"time"
)

func newListVMsCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "vms",
		Short: "List VMs",
		RunE:  runListVMs,
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

	table.AddRow("Name", "Created", "Image", "Status", "Restart policy")

	for _, vm := range vms {
		restartPolicyInfo := fmt.Sprintf("%s (%d restarts)", vm.RestartPolicy, vm.RestartCount)
		createdAtInfo := humanize.RelTime(vm.CreatedAt, time.Now(), "ago", "in the future")

		table.AddRow(vm.Name, createdAtInfo, vm.Image, vm.Status, restartPolicyInfo)
	}

	fmt.Println(table)

	return nil
}
