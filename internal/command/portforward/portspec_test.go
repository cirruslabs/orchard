package portforward_test

import (
	"github.com/cirruslabs/orchard/internal/command/portforward"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPortSpecNormal(t *testing.T) {
	portSpec, err := portforward.NewPortSpec("5555")
	require.NoError(t, err)
	require.Equal(t, &portforward.PortSpec{LocalPort: 0, RemotePort: 5555}, portSpec)

	portSpec, err = portforward.NewPortSpec("8000:80")
	require.NoError(t, err)
	require.Equal(t, &portforward.PortSpec{LocalPort: 8000, RemotePort: 80}, portSpec)
}

func TestPortSpecInvalid(t *testing.T) {
	_, err := portforward.NewPortSpec("")
	require.Error(t, err)

	_, err = portforward.NewPortSpec("0")
	require.Error(t, err)

	_, err = portforward.NewPortSpec("1:2:3")
	require.Error(t, err)
}
