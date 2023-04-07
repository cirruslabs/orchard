package pause

import "github.com/spf13/cobra"

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "pause",
		Short: "Pause a resource",
	}

	command.AddCommand(newPauseWorkerCommand())

	return command
}
