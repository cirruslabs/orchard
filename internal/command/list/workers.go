package list

import (
	"fmt"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/dustin/go-humanize"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"time"
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

	table.AddRow("Name", "Last seen", "Scheduling paused")

	for _, worker := range workers {
		lastSeenInfo := humanize.RelTime(worker.LastSeen, time.Now(), "ago", "in the future")

		table.AddRow(worker.Name, lastSeenInfo, worker.SchedulingPaused)
	}

	fmt.Println(table)

	return nil
}
