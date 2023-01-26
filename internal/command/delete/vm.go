package delete

import (
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/spf13/cobra"
)

func newDeleteVMCommand() *cobra.Command {
	return &cobra.Command{
		Use:  "vm",
		Args: cobra.ExactArgs(1),
		RunE: runDeleteVM,
	}
}

func runDeleteVM(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := client.New()
	if err != nil {
		return err
	}

	return client.VMs().Delete(cmd.Context(), name, false)
}
