package context

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/cirruslabs/orchard/internal/bootstraptoken"
	"github.com/cirruslabs/orchard/internal/certificatefingerprint"
	"github.com/cirruslabs/orchard/internal/config"
	"github.com/cirruslabs/orchard/internal/netconstants"
	clientpkg "github.com/cirruslabs/orchard/pkg/client"
	"github.com/manifoldco/promptui"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"net/url"
	"strconv"
)

var ErrCreateFailed = errors.New("failed to create context")

var bootstrapTokenRaw string
var serviceAccountName string
var serviceAccountToken string
var force bool
var noPKI bool

func newCreateCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "create NAME",
		Short: "Create a new context",
		Args:  cobra.ExactArgs(1),
		RunE:  runCreate,
	}

	command.PersistentFlags().StringVar(&contextName, "name", "default",
		"context name to use")
	command.PersistentFlags().StringVar(&bootstrapTokenRaw, "bootstrap-token", "",
		"bootstrap token to use")
	command.PersistentFlags().StringVar(&serviceAccountName, "service-account-name", "",
		"service account name to use (alternative to --bootstrap-token)")
	command.PersistentFlags().StringVar(&serviceAccountToken, "service-account-token", "",
		"service account token to use (alternative to --bootstrap-token)")
	command.PersistentFlags().BoolVar(&force, "force", false,
		"create the context even if a context with the same name already exists")
	command.PersistentFlags().BoolVar(&noPKI, "no-pki", false,
		"do not use the host's root CA set and instead validate the Controller's presented "+
			"certificate using a bootstrap token (or manually via fingerprint, "+
			"if no bootstrap token is provided)")

	return command
}

func runCreate(cmd *cobra.Command, args []string) error {
	controllerURL, err := netconstants.NormalizeAddress(args[0])
	if err != nil {
		return err
	}

	// If the bootstrap token is present, extract
	// service account credentials from it
	// and remember it for further use
	var bootstrapToken *bootstraptoken.BootstrapToken

	if bootstrapTokenRaw != "" {
		bootstrapToken, err = bootstraptoken.NewFromString(bootstrapTokenRaw)
		if err != nil {
			return err
		}

		if serviceAccountName == "" {
			serviceAccountName = bootstrapToken.ServiceAccountName()
		}
		if serviceAccountToken == "" {
			serviceAccountToken = bootstrapToken.ServiceAccountToken()
		}
	}

	trustedCertificate, err := tryToConnectToTheController(cmd.Context(), controllerURL, bootstrapToken)
	if err != nil {
		return err
	}

	// Create and save the context
	configHandle, err := config.NewHandle()
	if err != nil {
		return err
	}

	newContext := config.Context{
		URL:                 controllerURL.String(),
		ServiceAccountName:  serviceAccountName,
		ServiceAccountToken: serviceAccountToken,
	}

	if trustedCertificate != nil {
		certificatePEMBytes := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: trustedCertificate.Raw,
		})

		newContext.Certificate = certificatePEMBytes
	}

	return configHandle.CreateContext(contextName, newContext, force)
}

func tryToConnectToTheController(
	ctx context.Context,
	controllerURL *url.URL,
	bootstrapToken *bootstraptoken.BootstrapToken,
) (*x509.Certificate, error) {
	if !noPKI {
		fmt.Println("trying to connect to the controller using PKI and host's root CA set...")

		err := tryToConnectWithPKI(ctx, controllerURL)
		if err == nil {
			// Connection successful and no certificate retrieval is needed
			return nil, nil
		} else if errors.Is(err, clientpkg.ErrAPI) {
			// Makes no sense to go any further since it's an upper layer (HTTP, not TLS) error
			return nil, err
		}

		fmt.Printf("PKI association failed (%v), falling back to trusted-certificate approach...\n", err)
	}

	return tryToConnectWithTrustedCertificate(ctx, controllerURL, bootstrapToken)
}

func tryToConnectWithPKI(ctx context.Context, controllerURL *url.URL) error {
	client, err := clientpkg.New(
		clientpkg.WithAddress(controllerURL.String()),
		clientpkg.WithCredentials(serviceAccountName, serviceAccountToken),
	)
	if err != nil {
		return err
	}

	return client.Check(ctx)
}

func tryToConnectWithTrustedCertificate(
	ctx context.Context,
	controllerURL *url.URL,
	bootstrapToken *bootstraptoken.BootstrapToken,
) (*x509.Certificate, error) {
	// Either (1) retrieve a trusted certificate from the bootstrap token
	// or (2) retrieve it from the Controller and verify it interactively
	var trustedControllerCertificate *x509.Certificate
	var err error

	if bootstrapToken != nil {
		trustedControllerCertificate = bootstrapToken.Certificate()
	} else {
		if trustedControllerCertificate, err = probeControllerCertificate(ctx, controllerURL); err != nil {
			return nil, err
		}
	}

	// Now try again with the trusted certificate
	client, err := clientpkg.New(
		clientpkg.WithAddress(controllerURL.String()),
		clientpkg.WithCredentials(serviceAccountName, serviceAccountToken),
		clientpkg.WithTrustedCertificate(trustedControllerCertificate),
	)
	if err != nil {
		return nil, err
	}

	return trustedControllerCertificate, client.Check(ctx)
}

func probeControllerCertificate(ctx context.Context, controllerURL *url.URL) (*x509.Certificate, error) {
	//nolint:gosec // without InsecureSkipVerify our VerifyConnection won't be called
	insecureTLSConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	}

	var controllerCert *x509.Certificate

	insecureTLSConfig.VerifyConnection = func(state tls.ConnectionState) error {
		if controllerCert != nil {
			return fmt.Errorf("%w: encountered more than one certificate while probing the controller",
				ErrCreateFailed)
		}

		if len(state.PeerCertificates) == 0 {
			return fmt.Errorf("%w: controller presented no certificates, expected at least one",
				ErrCreateFailed)
		}

		// According to TLS 1.2[1] and TLS 1.3[2] specs:
		//
		// "The sender's certificate MUST come first in the list."
		//
		// [1]: https://www.rfc-editor.org/rfc/rfc5246#section-7.4.2
		// [2]: https://www.rfc-editor.org/rfc/rfc8446#section-4.4.2
		controllerCert = state.PeerCertificates[0]
		formattedControllerCertFingerprint := certificatefingerprint.CertificateFingerprint(controllerCert.Raw)

		shortControllerName := controllerURL.Hostname()
		if controllerURL.Port() != strconv.FormatUint(netconstants.DefaultControllerPort, 10) {
			shortControllerName += ":" + controllerURL.Port()
		}

		fmt.Printf("The authencity of controller %s cannot be established.\n", shortControllerName)
		fmt.Printf("Certificate SHA-256 fingerprint is: %s.\n", formattedControllerCertFingerprint)

		promptTemplates := &promptui.PromptTemplates{
			Prompt:          "{{ . }} ",
			Valid:           "{{ . }} ",
			Invalid:         "{{ . }} ",
			Success:         "{{ . }} ",
			ValidationError: "{{ . }} ",
		}
		prompt := promptui.Prompt{
			Label: "Are you sure you want to establish trust to this certificate? (yes/no)",
			Validate: func(s string) error {
				if s != "yes" && s != "no" {
					//nolint:goerr113,golint,stylecheck // this is not a standard error
					return fmt.Errorf("Please specify \"yes\" or \"no\".")
				}

				return nil
			},
			Templates: promptTemplates,
		}

		promptResult, err := prompt.Run()
		if err != nil {
			return err
		}

		switch promptResult {
		case "yes":
			return nil
		case "no":
			return fmt.Errorf("%w: certificate verification failed: no trust decision received from the user",
				ErrCreateFailed)
		default:
			return fmt.Errorf("%w: certificate verification failed: received unsupported answer from the user",
				ErrCreateFailed)
		}
	}

	dialer := tls.Dialer{
		Config: insecureTLSConfig,
	}

	conn, err := dialer.DialContext(ctx, "tcp", controllerURL.Host)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = conn.Close()
	}()

	return controllerCert, nil
}
