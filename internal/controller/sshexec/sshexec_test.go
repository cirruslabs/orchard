package sshexec_test

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/cirruslabs/orchard/internal/controller/sshexec"
	"github.com/stretchr/testify/require"
)

func TestContextCancellationViaNetConnClose(t *testing.T) {
	clientConn, serverConn := net.Pipe()

	go func() {
		select {
		case <-t.Context().Done():
			return
		case <-time.After(5 * time.Second):
			require.NoError(t, serverConn.Close())
		}
	}()

	_, err := sshexec.New(clientConn, "doesn't", "matter", false)
	require.Error(t, err)
}

func TestCommandWithEnvNoEnvLeavesCommandUnchanged(t *testing.T) {
	command, err := sshexec.CommandWithEnv("echo hello", nil)
	require.NoError(t, err)
	require.Equal(t, "echo hello", command)
}

func TestCommandWithEnvSortsAndQuotes(t *testing.T) {
	command, err := sshexec.CommandWithEnv("printf '%s|%s|%s' \"$GREETING\" \"$NAME\" \"$MULTILINE\"", map[string]string{
		"NAME":      "O'Reilly",
		"GREETING":  "hello $USER",
		"MULTILINE": "line 1\nline 2",
	})
	require.NoError(t, err)
	require.Equal(t, strings.Join([]string{
		"export GREETING='hello $USER'",
		"export MULTILINE='line 1",
		"line 2'",
		"export NAME='O'\\''Reilly'",
		"printf '%s|%s|%s' \"$GREETING\" \"$NAME\" \"$MULTILINE\"",
	}, "\n"), command)
}

func TestCommandWithEnvRejectsInvalidName(t *testing.T) {
	_, err := sshexec.CommandWithEnv("echo hello", map[string]string{
		"1INVALID": "value",
	})
	require.ErrorContains(t, err, "invalid environment variable name")
}

func TestCommandWithEnvRejectsNULValue(t *testing.T) {
	_, err := sshexec.CommandWithEnv("echo hello", map[string]string{
		"VALID": "bad\x00value",
	})
	require.ErrorContains(t, err, "contains NUL byte")
}
