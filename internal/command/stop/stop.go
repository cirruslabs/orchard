package stop

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "stop",
		Short: "Stop resources",
	}

	command.AddCommand(newStopVMCommand())

	return command
}
