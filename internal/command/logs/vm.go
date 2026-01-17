package logs

import (
	"fmt"
	"strings"

	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/spf13/cobra"
)

var logLimit int
var logOrder string

func newLogsVMCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "vm NAME",
		Short: "Retrieve VM logs",
		RunE:  runLogsVM,
		Args:  cobra.ExactArgs(1),
	}

	command.Flags().IntVar(&logLimit, "limit", 0, "Maximum number of log lines to return")
	command.Flags().StringVar(&logOrder, "order", "", "Sort order for log lines: asc or desc")

	return command
}

func runLogsVM(cmd *cobra.Command, args []string) error {
	name := args[0]

	apiClient, err := client.New()
	if err != nil {
		return err
	}

	options := client.LogsOptions{
		Limit: logLimit,
	}
	if logOrder != "" {
		order := strings.ToLower(logOrder)
		switch order {
		case string(client.LogsOrderAsc):
			options.Order = client.LogsOrderAsc
		case string(client.LogsOrderDesc):
			options.Order = client.LogsOrderDesc
		default:
			return fmt.Errorf("invalid order %q: expected asc or desc", logOrder)
		}
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
