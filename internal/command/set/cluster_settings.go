package set

import (
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/spf13/cobra"
)

var ErrClusterSettingsFailed = errors.New("failed to set cluster settings")

var hostDirPoliciesRaw []string

const hostDirPoliciesFlag = "host-dir-policies"

func newSetClusterSettingsCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "cluster-settings",
		Short: "Set cluster settings",
		RunE:  runSetClusterSettings,
	}

	command.PersistentFlags().StringSliceVar(&hostDirPoliciesRaw, hostDirPoliciesFlag, []string{},
		fmt.Sprintf("comma-separated list of hostDir policies containing an allowed path prefix "+
			"and an optional \":ro\" modifier to only allow read-only mounts for that path prefix "+
			"(for example, --%s=/Users/ci/sources:ro,/tmp)", hostDirPoliciesFlag))

	return command
}

func runSetClusterSettings(cmd *cobra.Command, args []string) error {
	// Convert arguments
	var hostDirPolicies []v1.HostDirPolicy

	for _, hostDirPolicyRaw := range hostDirPoliciesRaw {
		hostDirPolicy, err := v1.NewHostDirPolicyFromString(hostDirPolicyRaw)
		if err != nil {
			return err
		}

		hostDirPolicies = append(hostDirPolicies, hostDirPolicy)
	}

	// Check if we need to update anything in the cluster settings
	if !cmd.Flag(hostDirPoliciesFlag).Changed {
		return fmt.Errorf("%w: you need to specify at least one setting to update",
			ErrClusterSettingsFailed)
	}

	// Update cluster settings
	client, err := client.New()
	if err != nil {
		return err
	}

	clusterSettings, err := client.ClusterSettings().Get(cmd.Context())
	if err != nil {
		return err
	}

	if cmd.Flag(hostDirPoliciesFlag).Changed {
		clusterSettings.HostDirPolicies = hostDirPolicies
	}

	return client.ClusterSettings().Set(cmd.Context(), clusterSettings)
}
