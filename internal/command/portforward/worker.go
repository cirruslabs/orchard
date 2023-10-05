package portforward

import (
	"fmt"
	"github.com/cirruslabs/orchard/internal/proxy"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/spf13/cobra"
	"net"
)

func newPortForwardWorkerCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "worker WORKER_NAME [LOCAL_PORT]:REMOTE_PORT",
		Short: "Forward TCP port to the worker",
		Args:  cobra.ExactArgs(2),
		RunE:  runPortForwardWorkerCommand,
	}

	return command
}

func runPortForwardWorkerCommand(cmd *cobra.Command, args []string) (err error) {
	name := args[0]
	portSpecRaw := args[1]

	portSpec, err := NewPortSpec(portSpecRaw)
	if err != nil {
		return err
	}

	client, err := client.New()
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", portSpec.LocalPort))
	if err != nil {
		return err
	}
	defer func() {
		if listenerErr := listener.Close(); listenerErr != nil && err == nil {
			err = listenerErr
		}
	}()

	fmt.Printf("forwarding %s -> %s:%d...\n", listener.Addr(), name, portSpec.RemotePort)

	errCh := make(chan error, 1)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				errCh <- err

				return
			}

			go func() {
				defer conn.Close()

				wsConn, err := client.Workers().PortForward(cmd.Context(), name, portSpec.RemotePort)
				if err != nil {
					fmt.Printf("failed to forward port: %v\n", err)

					return
				}
				defer wsConn.Close()

				if err := proxy.Connections(wsConn, conn); err != nil {
					fmt.Printf("failed to forward port: %v\n", err)
				}
			}()
		}
	}()

	select {
	case <-cmd.Context().Done():
		return cmd.Context().Err()
	case err := <-errCh:
		return err
	}
}
