package create

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "create",
		Short: "Create resources on the controller (VMs)",
	}

	command.AddCommand(newCreateVMCommand())

	return command
}
