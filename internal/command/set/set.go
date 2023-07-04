package set

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "set",
		Short: "Set resource properties on the controller",
	}

	command.AddCommand(newSetClusterSettingsCommand())

	return command
}
