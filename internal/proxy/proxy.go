package proxy

import (
	"io"
	"net"
	"strings"
)

func Connections(left net.Conn, right net.Conn) (finalErr error) {
	leftErrCh := make(chan error, 1)
	rightErrCh := make(chan error, 1)

	recordErr := func(newErr error) {
		if newErr != nil && finalErr == nil {
			finalErr = newErr
		}
	}

	go func() {
		_, err := io.Copy(left, right)
		rightErrCh <- err
	}()

	go func() {
		_, err := io.Copy(right, left)
		leftErrCh <- err
	}()

	// Wait for some goroutine and then unlock the other goroutine
	// by closing its source io.Reader
	select {
	case err := <-rightErrCh:
		recordErr(err)
		recordErr(left.Close())
		recordErr(<-leftErrCh)
	case err := <-leftErrCh:
		recordErr(err)
		recordErr(right.Close())
		recordErr(<-rightErrCh)
	}

	if finalErr != nil && strings.Contains(finalErr.Error(), "use of closed network connection") {
		finalErr = nil
	}

	return finalErr
}
