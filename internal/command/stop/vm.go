package stop

import (
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/spf13/cobra"
)

func newStopVMCommand() *cobra.Command {
	return &cobra.Command{
		Use:  "vm",
		Args: cobra.ExactArgs(1),
		RunE: runStopVM,
	}
}

func runStopVM(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := client.New()
	if err != nil {
		return err
	}

	_, err = client.VMs().Stop(cmd.Context(), name)
	return err
}
