package bootstraptoken_test

import (
	"encoding/pem"
	"github.com/cirruslabs/orchard/internal/bootstraptoken"
	controllercmd "github.com/cirruslabs/orchard/internal/command/controller"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBootstrapTokenTwoWay(t *testing.T) {
	serviceAccountName := "admin"
	serviceAccountToken := uuid.New().String()

	tlsCert, err := controllercmd.GenerateSelfSignedControllerCertificate()
	require.NoError(t, err)

	block := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: tlsCert.Certificate[0],
	}
	certificatePEM := pem.EncodeToMemory(block)

	bootstrapTokenOld, err := bootstraptoken.New(certificatePEM, serviceAccountName, serviceAccountToken)
	require.NoError(t, err)

	bootstrapTokenNew, err := bootstraptoken.NewFromString(bootstrapTokenOld.String())
	require.NoError(t, err)

	require.Equal(t, bootstrapTokenOld.ServiceAccountName(), bootstrapTokenNew.ServiceAccountName())
	require.Equal(t, bootstrapTokenOld.ServiceAccountToken(), bootstrapTokenNew.ServiceAccountToken())
	require.Equal(t, bootstrapTokenOld.Certificate(), bootstrapTokenNew.Certificate())
}
