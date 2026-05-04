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

	_, err := sshexec.New(clientConn, "doesn't", "matter", sshexec.Options{})
	require.Error(t, err)
}

func TestCommandWithOptionsNoOptionsLeaveCommandUnchanged(t *testing.T) {
	command, err := sshexec.CommandWithOptions("echo hello", sshexec.Options{})
	require.NoError(t, err)
	require.Equal(t, "echo hello", command)
}

func TestCommandWithOptionsSortsAndQuotes(t *testing.T) {
	command, err := sshexec.CommandWithOptions(
		"printf '%s|%s|%s' \"$GREETING\" \"$NAME\" \"$MULTILINE\"",
		sshexec.Options{
			Workdir: "/tmp/a'b",
			Env: map[string]string{
				"NAME":      "O'Reilly",
				"GREETING":  "hello $USER",
				"MULTILINE": "line 1\nline 2",
			},
		},
	)
	require.NoError(t, err)
	require.Equal(t, strings.Join([]string{
		"cd '/tmp/a'\\''b' || exit $?",
		"export GREETING='hello $USER'",
		"export MULTILINE='line 1",
		"line 2'",
		"export NAME='O'\\''Reilly'",
		"printf '%s|%s|%s' \"$GREETING\" \"$NAME\" \"$MULTILINE\"",
	}, "\n"), command)
}

func TestCommandWithOptionsRejectsInvalidName(t *testing.T) {
	_, err := sshexec.CommandWithOptions("echo hello", sshexec.Options{
		Env: map[string]string{
			"1INVALID": "value",
		},
	})
	require.ErrorContains(t, err, "invalid environment variable name")
}

func TestCommandWithOptionsRejectsNULValue(t *testing.T) {
	_, err := sshexec.CommandWithOptions("echo hello", sshexec.Options{
		Env: map[string]string{
			"VALID": "bad\x00value",
		},
	})
	require.ErrorContains(t, err, "contains NUL byte")
}

func TestCommandWithOptionsRejectsNULWorkdir(t *testing.T) {
	_, err := sshexec.CommandWithOptions("echo hello", sshexec.Options{
		Workdir: "bad\x00dir",
	})
	require.ErrorContains(t, err, "working directory contains NUL byte")
}
