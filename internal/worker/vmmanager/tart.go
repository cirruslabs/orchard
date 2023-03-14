package vmmanager

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const tartCommandName = "tart"

var (
	ErrTartNotFound = errors.New("tart command not found")
	ErrTartFailed   = errors.New("tart command returned non-zero exit code")
)

//nolint:unparam // might use stderr at some point
func (vm *VM) tart(
	ctx context.Context,
	args ...string,
) (string, string, error) {
	cmd := exec.CommandContext(ctx, tartCommandName, args...)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	vm.logger.Debugf("running '%s %s'", tartCommandName, strings.Join(args, " "))
	err := cmd.Run()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", "", fmt.Errorf("%w: %s command not found in PATH, make sure Tart is installed",
				ErrTartNotFound, tartCommandName)
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			vm.logger.Warnf(
				"'%s %s' failed with exit code %d: %s",
				tartCommandName, strings.Join(args, " "),
				exitErr.ExitCode(), firstNonEmptyLine(stderr.String(), stdout.String()),
			)
			// Tart command failed, redefine the error
			// to be the Tart-specific output
			err = fmt.Errorf("%w: %q", ErrTartFailed, firstNonEmptyLine(stderr.String(), stdout.String()))
		}
	}

	return stdout.String(), stderr.String(), err
}

func firstNonEmptyLine(outputs ...string) string {
	for _, output := range outputs {
		for _, line := range strings.Split(output, "\n") {
			if line != "" {
				return line
			}
		}
	}

	return ""
}
