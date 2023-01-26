package list

import (
	"fmt"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/dustin/go-humanize"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
)

func newListWorkersCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "workers",
		Short: "List workers",
		RunE:  runListWorkers,
	}

	return command
}

func runListWorkers(cmd *cobra.Command, args []string) error {
	client, err := client.New()
	if err != nil {
		return err
	}

	workers, err := client.Workers().List(cmd.Context())
	if err != nil {
		return err
	}

	if quiet {
		for _, worker := range workers {
			fmt.Println(worker.Name)
		}

		return nil
	}

	table := uitable.New()

	table.AddRow("Name", "Last seen")

	for _, worker := range workers {
		table.AddRow(worker.Name, humanize.Time(worker.LastSeen))
	}

	fmt.Println(table)

	return nil
}
