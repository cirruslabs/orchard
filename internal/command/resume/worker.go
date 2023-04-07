package resume

import (
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/spf13/cobra"
)

func newResumeWorkerCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "worker NAME",
		Short: "Resume a worker",
		RunE:  runResumeWorker,
		Args:  cobra.ExactArgs(1),
	}

	return command
}

func runResumeWorker(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := client.New()
	if err != nil {
		return err
	}

	worker, err := client.Workers().Get(cmd.Context(), name)
	if err != nil {
		return err
	}

	if worker.Unschedulable {
		worker.Unschedulable = false

		_, err = client.Workers().Update(cmd.Context(), *worker)
		if err != nil {
			return err
		}
	}

	return nil
}
