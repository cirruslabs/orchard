package controller

import (
	"crypto/tls"
	"fmt"
	"github.com/cirruslabs/orchard/internal/certificatefingerprint"
	"github.com/cirruslabs/orchard/internal/controller"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/pterm/pterm"
	"github.com/sethvargo/go-password/password"
	"os"
)

const BootstrapAdminName = "bootstrap-admin"

func Bootstrap(controllerInstance *controller.Controller, controllerCert tls.Certificate) error {
	// Determine if we need to do anything at all
	orchardBootstrapAdminToken, orchardBootstrapAdminTokenPresent := os.LookupEnv("ORCHARD_BOOTSTRAP_ADMIN_TOKEN")

	numServiceAccounts, err := controllerInstance.NumServiceAccounts()
	if err != nil {
		return fmt.Errorf("failed to retrieve the number of service accounts: %w", err)
	}

	if numServiceAccounts != 0 && !orchardBootstrapAdminTokenPresent {
		// No bootstrap is needed because there are service accounts
		// present in the database (so it's not the first start)
		// and no bootstrap admin token change is requested
		return nil
	}

	// Generate a bootstrap admin token if not present in the environment variable
	if !orchardBootstrapAdminTokenPresent {
		orchardBootstrapAdminToken, err = password.Generate(32, 10, 10,
			false, false)
		if err != nil {
			return fmt.Errorf("failed to generate bootstrap admin token: %w", err)
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
		return err
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

	pterm.Info.Print(
		pterm.Sprintf("%s:\n", reasonToDisplay),
		pterm.Sprintln(),
		pterm.Sprintf("Service account name: %s\n", pterm.Bold.Sprint(BootstrapAdminName)),
		pterm.Sprintf("Service account token: %s\n", pterm.Bold.Sprint(serviceAccountTokenToDisplay)),
		pterm.Sprintf("Certificate SHA-256 fingerprint: %s.\n",
			pterm.Bold.Sprint(certificatefingerprint.CertificateFingerprint(controllerCert.Certificate[0]))),
	)

	return nil
}
