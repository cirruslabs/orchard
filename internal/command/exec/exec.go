package exec

import "github.com/spf13/cobra"

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "exec",
		Short: "Execute commands inside resources",
	}

	command.AddCommand(newExecVMCommand())

	return command
}
