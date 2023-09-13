package get

import (
	"errors"
	"github.com/spf13/cobra"
)

var ErrGetFailed = errors.New("get command failed")

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "get",
		Short: "Retrieve resources from the controller",
	}

	command.AddCommand(
		newGetBootstrapTokenCommand(),
		newGetClusterSettingsCommand(),
		newGetServiceAccountCommand(),
		newGetVMCommand(),
		newGetWorkerCommand(),
	)

	return command
}

func nonEmptyOrNone(s string) string {
	if s != "" {
		return s
	}

	return "none"
}
