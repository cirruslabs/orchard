package deletecmd

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "delete",
		Short: "Delete resources from the controller",
	}

	command.AddCommand(newDeleteVMCommand(), newDeleteServiceComandCommand(), newDeleteWorkerCommand())

	return command
}
