package start

import (
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/spf13/cobra"
)

func newStartVMCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "vm NAME",
		Short: "Start a VM",
		Args:  cobra.ExactArgs(1),
		RunE:  runStartVM,
	}
}

func runStartVM(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := client.New()
	if err != nil {
		return err
	}

	_, err = client.VMs().Start(cmd.Context(), name)
	return err
}
