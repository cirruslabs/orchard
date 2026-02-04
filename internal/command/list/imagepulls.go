package list

import (
	"fmt"

	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
)

func newListImagePullsCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "imagepulls",
		Short: "List image pulls",
		RunE:  runListImagePulls,
	}

	return command
}

func runListImagePulls(cmd *cobra.Command, args []string) error {
	client, err := client.New()
	if err != nil {
		return err
	}

	imagePulls, err := client.ImagePulls().List(cmd.Context())
	if err != nil {
		return err
	}

	if quiet {
		for _, imagePull := range imagePulls {
			fmt.Println(imagePull.Name)
		}

		return nil
	}

	table := uitable.New()
	table.Wrap = true

	table.AddRow("Name", "Image", "Worker", "Conditions")

	for _, imagePullJob := range imagePulls {
		table.AddRow(imagePullJob.Name, imagePullJob.Image, imagePullJob.Worker,
			v1.ConditionsHumanize(imagePullJob.Conditions))
	}

	fmt.Println(table)

	return nil
}
