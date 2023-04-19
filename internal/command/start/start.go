package start

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "start",
		Short: "Start resources",
	}

	command.AddCommand(newStartVMCommand())

	return command
}
