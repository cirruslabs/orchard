package deletecmd

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "delete",
		Short: "Delete resources from the controller (VMs)",
	}

	command.AddCommand(newDeleteVMCommand())

	return command
}
