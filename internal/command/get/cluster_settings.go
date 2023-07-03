package get

import (
	"fmt"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/gosuri/uitable"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"strings"
)

func newGetClusterSettingsCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "cluster-settings",
		Short: "Retrieve cluster settings",
		RunE:  runGetClusterSettings,
	}

	return command
}

func runGetClusterSettings(cmd *cobra.Command, args []string) error {
	client, err := client.New()
	if err != nil {
		return err
	}

	clusterSettings, err := client.ClusterSettings().Get(cmd.Context())
	if err != nil {
		return err
	}

	table := uitable.New()

	table.AddRow("Key", "Value")

	hostDirPoliciesAsStrings := lo.Map(clusterSettings.HostDirPolicies, func(policy v1.HostDirPolicy, _ int) string {
		return policy.String()
	})
	hostDirPoliciesDescription := strings.Join(hostDirPoliciesAsStrings, ",")
	if hostDirPoliciesDescription == "" {
		hostDirPoliciesDescription = "none"
	}
	table.AddRow("hostDir policies", hostDirPoliciesDescription)

	fmt.Println(table)

	return nil
}
