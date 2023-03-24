package deletecmd

import (
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/spf13/cobra"
)

func newDeleteWorkerCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "worker NAME",
		Short: "Delete a worker",
		Args:  cobra.ExactArgs(1),
		RunE:  runDeleteWorker,
	}
}

func runDeleteWorker(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := client.New()
	if err != nil {
		return err
	}

	return client.Workers().Delete(cmd.Context(), name)
}
