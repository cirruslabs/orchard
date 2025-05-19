package list

import (
	"github.com/spf13/cobra"
)

var quiet bool

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "list",
		Short: "List resources on the controller",
	}

	command.AddCommand(newListWorkersCommand(), newListVMsCommand(), newListServiceAccountsCommand())

	command.Flags().BoolVarP(&quiet, "", "q", false, "only show resource names")

	return command
}
