package pause

import (
	"context"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"time"
)

var wait uint64

func newPauseWorkerCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "worker NAME",
		Short: "Pause a worker",
		RunE:  runPauseWorker,
		Args:  cobra.ExactArgs(1),
	}

	command.PersistentFlags().Uint64Var(&wait, "wait", 0,
		"wait the specified amount of seconds for the worker to stop running any VMs")

	return command
}

func runPauseWorker(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := client.New()
	if err != nil {
		return err
	}

	worker, err := client.Workers().Get(cmd.Context(), name)
	if err != nil {
		return err
	}

	if !worker.SchedulingPaused {
		worker.SchedulingPaused = true

		_, err = client.Workers().Update(cmd.Context(), *worker)
		if err != nil {
			return err
		}
	}

	if wait != 0 {
		subCtx, cancel := context.WithTimeout(cmd.Context(), time.Duration(wait)*time.Second)
		defer cancel()

		for {
			vms, err := client.VMs().FindForWorker(cmd.Context(), worker.Name)
			if err != nil {
				return err
			}

			hasActiveVMs := slices.ContainsFunc(maps.Values(vms), func(vm v1.VM) bool {
				return !vm.TerminalState()
			})
			if !hasActiveVMs {
				break
			}

			select {
			case <-time.After(time.Second):
				continue
			case <-subCtx.Done():
				return subCtx.Err()
			}
		}
	}

	return nil
}
