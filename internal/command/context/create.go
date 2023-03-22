package context

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/bootstraptoken"
	"github.com/cirruslabs/orchard/internal/config"
	"github.com/cirruslabs/orchard/internal/netconstants"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"net/url"
	"strconv"
	"strings"
)

var ErrCreateFailed = errors.New("failed to create context")

var bootstrapTokenRaw string
var serviceAccountName string
var serviceAccountToken string
var force bool

func newCreateCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "create",
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

	return command
}

func runCreate(cmd *cobra.Command, args []string) error {
	controllerURL, err := netconstants.NormalizeAddress(args[0])
	if err != nil {
		return err
	}

	// Establish trust
	var trustedControllerCertificate *x509.Certificate

	if bootstrapTokenRaw != "" {
		bootstrapToken, err := bootstraptoken.NewFromString(bootstrapTokenRaw)
		if err != nil {
			return err
		}

		serviceAccountName = bootstrapToken.ServiceAccountName()
		serviceAccountToken = bootstrapToken.ServiceAccountToken()
		trustedControllerCertificate = bootstrapToken.Certificate()
	} else {
		trustedControllerCertificate, err = probeControllerCertificate(controllerURL)
		if err != nil {
			return err
		}
	}

	client, err := client.New(
		client.WithAddress(controllerURL.String()),
		client.WithTrustedCertificate(trustedControllerCertificate),
		client.WithCredentials(serviceAccountName, serviceAccountToken),
	)
	if err != nil {
		return err
	}
	if err := client.Check(cmd.Context()); err != nil {
		return err
	}

	// Create and save the context
	configHandle, err := config.NewHandle()
	if err != nil {
		return err
	}

	certificatePEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: trustedControllerCertificate.Raw,
	})

	return configHandle.CreateContext(contextName, config.Context{
		URL:                 controllerURL.String(),
		Certificate:         certificatePEMBytes,
		ServiceAccountName:  serviceAccountName,
		ServiceAccountToken: serviceAccountToken,
	}, force)
}

func probeControllerCertificate(controllerURL *url.URL) (*x509.Certificate, error) {
	// Do not use PKI
	emptyPool := x509.NewCertPool()

	//nolint:gosec // since we're not using PKI, InsecureSkipVerify is a must here
	insecureTLSConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		RootCAs:            emptyPool,
		ServerName:         netconstants.DefaultControllerServerName,
		InsecureSkipVerify: true,
	}

	var controllerCert *x509.Certificate

	insecureTLSConfig.VerifyConnection = func(state tls.ConnectionState) error {
		if controllerCert != nil {
			return fmt.Errorf("%w: encountered more than one certificate while probing the controller",
				ErrCreateFailed)
		}

		if len(state.PeerCertificates) != 1 {
			return fmt.Errorf("%w: controller presented %d certificate(s), expected only one",
				ErrCreateFailed, len(state.PeerCertificates))
		}

		controllerCert = state.PeerCertificates[0]
		controllerCertFingerprint := sha256.Sum256(controllerCert.Raw)
		formattedControllerCertFingerprint := formatFingerprint(controllerCertFingerprint[:])

		shortControllerName := controllerURL.Hostname()
		if controllerURL.Port() != strconv.FormatUint(netconstants.DefaultControllerPort, 10) {
			shortControllerName += ":" + controllerURL.Port()
		}

		fmt.Printf("The authencity of controller %s cannot be established.\n", shortControllerName)
		fmt.Printf("Certificate SHA-256 fingerprint is %s.\n", formattedControllerCertFingerprint)

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

	conn, err := tls.Dial("tcp", controllerURL.Host, insecureTLSConfig)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = conn.Close()
	}()

	return controllerCert, nil
}

func formatFingerprint(fingerprint []byte) string {
	var fingerprintPieces []string

	for _, piece := range fingerprint {
		fingerprintPieces = append(fingerprintPieces, fmt.Sprintf("%02X", piece))
	}

	return strings.Join(fingerprintPieces, " ")
}
