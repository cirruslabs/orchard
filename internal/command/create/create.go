package create

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "create",
		Short: "Create resources on the controller",
	}

	command.AddCommand(newCreateVMCommand(), newCreateServiceAccount())

	return command
}
