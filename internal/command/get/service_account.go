package get

import (
	"fmt"
	"github.com/cirruslabs/orchard/internal/structpath"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"strings"
)

func newGetServiceAccountCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "service-account NAME",
		Short: "Retrieve a service account and it's fields",
		RunE:  runGetServiceAccount,
		Args:  cobra.ExactArgs(1),
	}

	return command
}

func runGetServiceAccount(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := client.New()
	if err != nil {
		return err
	}

	// Ability to retrieve resource fields (e.g. "orchard get service-account workers/token")
	splits := strings.Split(name, "/")
	var path []string
	if len(splits) > 1 {
		name = splits[0]
		path = splits[1:]
	}

	serviceAccount, err := client.ServiceAccounts().Get(cmd.Context(), name)
	if err != nil {
		return err
	}

	// Ability to retrieve resource fields (e.g. "orchard get service-account workers/token")
	if len(path) != 0 {
		result, ok := structpath.Lookup(*serviceAccount, path)
		if !ok {
			return fmt.Errorf("%w: failed to find the specified field \"%s\" or the field is not a string",
				ErrGetFailed, strings.Join(path, "/"))
		}

		fmt.Println(result)

		return nil
	}

	table := uitable.New()

	table.AddRow("name", serviceAccount.Name)

	var scopeList []string
	for _, scope := range serviceAccount.Roles {
		scopeList = append(scopeList, string(scope))
	}
	table.AddRow("roles", strings.Join(scopeList, ", "))

	fmt.Println(table)

	return nil
}
