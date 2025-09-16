package list

import (
	"fmt"
	"strings"

	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
)

func newListServiceAccountsCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "service-accounts",
		Short: "List service accounts",
		RunE:  runListServiceAccounts,
	}

	return command
}

func runListServiceAccounts(cmd *cobra.Command, args []string) error {
	client, err := client.New()
	if err != nil {
		return err
	}

	serviceAccounts, err := client.ServiceAccounts().List(cmd.Context())
	if err != nil {
		return err
	}

	if quiet {
		for _, serviceAccount := range serviceAccounts {
			fmt.Println(serviceAccount.Name)
		}

		return nil
	}

	table := uitable.New()
	table.Wrap = true

	table.AddRow("Name", "Roles")

	for _, serviceAccount := range serviceAccounts {
		var scopeList []string

		for _, scope := range serviceAccount.Roles {
			scopeList = append(scopeList, string(scope))
		}

		table.AddRow(serviceAccount.Name, strings.Join(scopeList, ", "))
	}

	fmt.Println(table)

	return nil
}
