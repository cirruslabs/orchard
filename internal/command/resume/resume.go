package resume

import "github.com/spf13/cobra"

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "resume",
		Short: "Resume a resource",
	}

	command.AddCommand(newResumeWorkerCommand())

	return command
}
