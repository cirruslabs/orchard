package list

import (
	"fmt"

	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
)

func newListImagePullJobsCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "imagepulljobs",
		Short: "List image pull jobs",
		RunE:  runListImagePullJobs,
	}

	return command
}

func runListImagePullJobs(cmd *cobra.Command, args []string) error {
	client, err := client.New()
	if err != nil {
		return err
	}

	imagePullJobs, err := client.ImagePullJobs().List(cmd.Context())
	if err != nil {
		return err
	}

	if quiet {
		for _, imagePullJob := range imagePullJobs {
			fmt.Println(imagePullJob.Name)
		}

		return nil
	}

	table := uitable.New()
	table.Wrap = true

	table.AddRow("Name", "Image", "Labels", "Progressing", "Succeeded", "Failed", "Total")

	for _, imagePullJob := range imagePullJobs {
		table.AddRow(imagePullJob.Name, imagePullJob.Image, imagePullJob.Labels, imagePullJob.Progressing,
			imagePullJob.Succeeded, imagePullJob.Failed, imagePullJob.Total)
	}

	fmt.Println(table)

	return nil
}
