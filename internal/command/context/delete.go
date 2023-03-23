package context

import (
	"github.com/cirruslabs/orchard/internal/config"
	"github.com/spf13/cobra"
)

func newDeleteCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "delete NAME",
		Short: "Delete a context",
		Args:  cobra.ExactArgs(1),
		RunE:  runDelete,
	}

	return command
}

func runDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	configHandle, err := config.NewHandle()
	if err != nil {
		return err
	}

	return configHandle.DeleteContext(name)
}
