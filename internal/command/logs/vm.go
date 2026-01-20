package logs

import (
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/spf13/cobra"
)

var logTail int

func newLogsVMCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "vm NAME",
		Short: "Retrieve VM logs",
		RunE:  runLogsVM,
		Args:  cobra.ExactArgs(1),
	}

	command.Flags().IntVar(&logTail, "tail", 0, "Number of log lines to show from the end (newest first)")

	return command
}

func runLogsVM(cmd *cobra.Command, args []string) error {
	name := args[0]

	apiClient, err := client.New()
	if err != nil {
		return err
	}

	options := client.LogsOptions{}
	if logTail > 0 {
		options.Limit = logTail
		options.Order = client.LogsOrderDesc
	}

	lines, err := apiClient.VMs().LogsWithOptions(cmd.Context(), name, options)
	if err != nil {
		return err
	}

	for _, line := range lines {
		fmt.Println(line)
	}

	return nil
}
