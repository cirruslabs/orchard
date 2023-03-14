package proxy

import (
	"io"
	"net"
)

func Connections(left net.Conn, right net.Conn) error {
	errCh := make(chan error, 2)

	go func() {
		_, err := io.Copy(left, right)
		errCh <- err
	}()

	go func() {
		_, err := io.Copy(right, left)
		errCh <- err
	}()

	firstErr := <-errCh

	// Force connection closure to unlock the other goroutine
	left.Close()
	right.Close()

	secondErr := <-errCh

	if firstErr != nil {
		return firstErr
	}

	return secondErr
}
