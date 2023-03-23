package vnc

import (
	"fmt"
	"github.com/cirruslabs/orchard/internal/command/ssh"
	"github.com/cirruslabs/orchard/internal/proxy"
	"github.com/cirruslabs/orchard/pkg/client"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
	"net"
)

const vncPort = 5900

var username string
var password string

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "vnc VM_NAME",
		Short: "Open VNC session with the VM",
		Args:  cobra.ExactArgs(1),
		RunE:  run,
	}

	command.PersistentFlags().StringVarP(&username, "username", "u", "",
		"VNC username")
	command.PersistentFlags().StringVarP(&password, "password", "p", "",
		"VNC password")

	return command
}

func run(cmd *cobra.Command, args []string) (err error) {
	name := args[0]

	client, err := client.New()
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer func() {
		if listenerErr := listener.Close(); listenerErr != nil && err == nil {
			err = listenerErr
		}
	}()

	fmt.Printf("forwarding %s -> %s:%d...\n", listener.Addr(), name, vncPort)

	errCh := make(chan error, 1)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				errCh <- err

				return
			}

			go func() {
				wsConn, err := client.VMs().PortForward(cmd.Context(), name, vncPort)
				if err != nil {
					fmt.Printf("failed to forward port: %v\n", err)

					return
				}

				if err := proxy.Connections(wsConn, conn); err != nil {
					fmt.Printf("failed to forward port: %v\n", err)
				}
			}()
		}
	}()

	username, password = ssh.ChooseUsernameAndPassword(cmd.Context(), client, name, username, password)

	openURL := fmt.Sprintf("vnc://%s:%s@%s", username, password, listener.Addr().String())
	openURLSanitized := fmt.Sprintf("vnc://%s@%s", username, listener.Addr().String())

	fmt.Printf("opening %s...\n", openURLSanitized)

	if err := open.Start(openURL); err != nil {
		fmt.Printf("failed to open: %v\n", err)
	}

	select {
	case <-cmd.Context().Done():
		return cmd.Context().Err()
	case err := <-errCh:
		return err
	}
}
