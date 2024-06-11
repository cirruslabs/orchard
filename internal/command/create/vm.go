package create

import (
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/simplename"
	"github.com/cirruslabs/orchard/pkg/client"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var ErrVMFailed = errors.New("failed to create VM")

var image string
var cpu uint64
var memory uint64
var netSoftnet bool
var netBridged string
var headless bool
var username string
var password string
var resources map[string]string
var restartPolicy string
var startupScript string
var hostDirsRaw []string
var imagePullPolicy string

func newCreateVMCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "vm NAME",
		Short: "Create a VM",
		RunE:  runCreateVM,
		Args:  cobra.ExactArgs(1),
	}

	command.PersistentFlags().StringVar(&image, "image", "ghcr.io/cirruslabs/macos-sonoma-base:latest", "image to use")
	command.PersistentFlags().Uint64Var(&cpu, "cpu", 4, "number of CPUs to use")
	command.PersistentFlags().Uint64Var(&memory, "memory", 8*1024, "megabytes of memory to use")
	command.PersistentFlags().BoolVar(&netSoftnet, "net-softnet", false, "whether to use Softnet network isolation")
	command.PersistentFlags().StringVar(&netBridged, "net-bridged", "", "whether to use Bridged network mode")
	command.PersistentFlags().BoolVar(&headless, "headless", true, "whether to run without graphics")
	command.PersistentFlags().StringVar(&username, "username", "admin",
		"SSH username to use when executing a startup script on the VM")
	command.PersistentFlags().StringVar(&password, "password", "admin",
		"SSH password to use when executing a startup script on the VM")
	command.PersistentFlags().StringToStringVar(&resources, "resources", map[string]string{},
		"resources to request for this VM")
	command.PersistentFlags().StringVar(&restartPolicy, "restart-policy", string(v1.RestartPolicyNever),
		fmt.Sprintf("restart policy for this VM: specify %q to never restart or %q "+
			"to only restart when the VM fails", v1.RestartPolicyNever, v1.RestartPolicyOnFailure))
	command.PersistentFlags().StringVar(&startupScript, "startup-script", "",
		"startup script (e.g. --startup-script=\"sync\") or a path to a script file prefixed with \"@\" "+
			"(e.g. \"--startup-script=@script.sh\")")
	command.PersistentFlags().StringSliceVar(&hostDirsRaw, "host-dirs", []string{},
		"host directories to mount to the VM, can be specified multiple times and/or be comma-separated "+
			"(see \"tart run\"'s --dir argument for syntax)")
	command.PersistentFlags().StringVar(&imagePullPolicy, "image-pull-policy", string(v1.ImagePullPolicyIfNotPresent),
		fmt.Sprintf("image pull policy for this VM, by default the image is only pulled if it doesn't "+
			"exist in the cache (%q), specify %q to always try to pull the image",
			v1.ImagePullPolicyIfNotPresent, v1.ImagePullPolicyAlways))

	return command
}

func runCreateVM(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Issue a warning if the name used will be invalid in the future
	if err := simplename.ValidateNext(name); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "WARNING: %v\n", err)
	}

	// Convert arguments
	var hostDirs []v1.HostDir

	for _, hostDirRaw := range hostDirsRaw {
		hostDir, err := v1.NewHostDirFromString(hostDirRaw)
		if err != nil {
			return err
		}

		hostDirs = append(hostDirs, hostDir)
	}

	vm := &v1.VM{
		Meta: v1.Meta{
			Name: name,
		},
		Image:      image,
		CPU:        cpu,
		Memory:     memory,
		NetSoftnet: netSoftnet,
		NetBridged: netBridged,
		Headless:   headless,
		Username:   username,
		Password:   password,
		HostDirs:   hostDirs,
	}

	// Convert resources
	var err error

	vm.Resources, err = v1.NewResourcesFromStringToString(resources)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrVMFailed, err)
	}

	// Convert image pull policy
	vm.ImagePullPolicy, err = v1.NewImagePullPolicyFromString(imagePullPolicy)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrVMFailed, err)
	}

	// Convert restart policy
	vm.RestartPolicy, err = v1.NewRestartPolicyFromString(restartPolicy)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrVMFailed, err)
	}

	// Convert startup script, optionally reading it from the file system
	const scriptFilePrefix = "@"

	if strings.HasPrefix(startupScript, scriptFilePrefix) {
		startupScriptBytes, err := os.ReadFile(strings.TrimPrefix(startupScript, scriptFilePrefix))
		if err != nil {
			return err
		}

		vm.StartupScript = &v1.VMScript{
			ScriptContent: string(startupScriptBytes),
		}
	} else if startupScript != "" {
		vm.StartupScript = &v1.VMScript{
			ScriptContent: startupScript,
		}
	}

	client, err := client.New()
	if err != nil {
		return err
	}

	return client.VMs().Create(cmd.Context(), vm)
}
