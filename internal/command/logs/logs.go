package logs

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "logs",
		Short: "Retrieve resource logs from the controller",
	}

	command.AddCommand(newLogsVMCommand())

	return command
}
