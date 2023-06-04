package logs

import (
	"fmt"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/spf13/cobra"
)

func newLogsVMCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "vm NAME",
		Short: "Retrieve VM logs",
		RunE:  runLogsVM,
		Args:  cobra.ExactArgs(1),
	}

	return command
}

func runLogsVM(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := client.New()
	if err != nil {
		return err
	}

	lines, err := client.VMs().Logs(cmd.Context(), name)
	if err != nil {
		return err
	}

	for _, line := range lines {
		fmt.Println(line)
	}

	return nil
}
