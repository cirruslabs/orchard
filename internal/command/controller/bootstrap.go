package controller

import (
	"crypto/tls"
	"fmt"
	"github.com/cirruslabs/orchard/internal/certificatefingerprint"
	"github.com/cirruslabs/orchard/internal/controller"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/sethvargo/go-password/password"
	"os"
	"strings"
)

const BootstrapContextName = "boostrap-context"
const BootstrapAdminName = "bootstrap-admin"

func Bootstrap(controllerInstance *controller.Controller, controllerCert tls.Certificate) (string, string, error) {
	// Determine if we need to do anything at all
	orchardBootstrapAdminToken, orchardBootstrapAdminTokenPresent := os.LookupEnv("ORCHARD_BOOTSTRAP_ADMIN_TOKEN")

	serviceAccounts, err := controllerInstance.ServiceAccounts()
	if err != nil {
		return "", "", fmt.Errorf("failed to retrieve the number of service accounts: %w", err)
	}

	if len(serviceAccounts) != 0 && !orchardBootstrapAdminTokenPresent {
		// No bootstrap is needed because there are service accounts
		// present in the database (so it's not the first start)
		// and no bootstrap admin token change is requested
		//
		// However, if the BootstrapAdminName service account still exists,
		// return its credentials. We'll use them for updating the
		// BootstrapContextName context.
		if serviceAccount, ok := lo.Find(serviceAccounts, func(serviceAccount *v1.ServiceAccount) bool {
			return serviceAccount.Name == BootstrapAdminName
		}); ok {
			return serviceAccount.Name, serviceAccount.Token, nil
		}

		return "", "", nil
	}

	// Generate a bootstrap admin token if not present in the environment variable
	if !orchardBootstrapAdminTokenPresent {
		passwordGenerator, err := password.NewGenerator(&password.GeneratorInput{
			LowerLetters: password.LowerLetters,
			UpperLetters: password.UpperLetters,
			Digits:       password.Digits,
			Symbols: strings.Map(func(r rune) rune {
				// Avoid generating $ and " symbols
				// as they cause issues in shell
				switch r {
				case '$', '"':
					return -1
				default:
					return r
				}
			}, password.Symbols),
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to generate bootstrap admin token: "+
				"failed to initialize password generator: %w", err)
		}

		orchardBootstrapAdminToken, err = passwordGenerator.Generate(32, 10, 10,
			false, false)
		if err != nil {
			return "", "", fmt.Errorf("failed to generate bootstrap admin token: %w", err)
		}
	}

	// Ensure bootstrap admin service account exists
	if err := controllerInstance.EnsureServiceAccount(&v1.ServiceAccount{
		Meta: v1.Meta{
			Name: BootstrapAdminName,
		},
		Token: orchardBootstrapAdminToken,
		Roles: v1.AllServiceAccountRoles(),
	}); err != nil {
		return "", "", err
	}

	// Report bootstrapping result to the user
	var reasonToDisplay string
	var serviceAccountTokenToDisplay string

	if orchardBootstrapAdminTokenPresent {
		reasonToDisplay = fmt.Sprintf("Re-created the %q service account using the token "+
			"provided in the ORCHARD_BOOTSTRAP_ADMIN_TOKEN environment variable", BootstrapAdminName)
		serviceAccountTokenToDisplay = "<hidden>"
	} else {
		reasonToDisplay = fmt.Sprintf("No service accounts found, created a new %q service "+
			"account using a randomly-generated password", BootstrapAdminName)
		serviceAccountTokenToDisplay = orchardBootstrapAdminToken
	}

	messages := []any{
		pterm.Sprintf("%s:\n", reasonToDisplay),
		pterm.Sprintln(),
		pterm.Sprintf("Service account name: %s\n", pterm.Bold.Sprint(BootstrapAdminName)),
		pterm.Sprintf("Service account token: %s\n", pterm.Bold.Sprint(serviceAccountTokenToDisplay)),
	}

	if !noTLS {
		messages = append(messages, pterm.Sprintf("Certificate SHA-256 fingerprint: %s.\n",
			pterm.Bold.Sprint(certificatefingerprint.CertificateFingerprint(controllerCert.Certificate[0]))))
	}

	pterm.Info.Print(messages...)

	return BootstrapAdminName, orchardBootstrapAdminToken, nil
}
