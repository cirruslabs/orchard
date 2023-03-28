# Orchard

Orchard is an orchestration system for [Tart](https://github.com/cirruslabs/tart).

Create a cluster of bare-metal Apple Silicon machines and manage dozens of VMs with ease!

## Quick start

Start the Orchard in local development mode:

```shell
brew install cirruslabs/cli/orchard
orchard dev
```

This will start Orchard Controller and a single Orchard Worker on your local machine.
For production deployments, please refer to the [Deployment Guide](./DeploymentGuide.md).

### Creating Virtual Machines

Create a Virtual Machine resource:

```shell
orchard create vm --image ghcr.io/cirruslabs/macos-ventura-base:latest ventura-base
```

Check a list of VM resources to see if the Virtual Machine we've created above is already running: 

```shell
orchard list vms
```

### Accessing Virtual Machines

To SSH into a VM use the `orchard ssh` command:

```shell
orchard ssh vm ventura-base
```

You can specify the `--username` and `--passowrd` flags to specify the username/password pair to SSH. By default, `admin`/`admin` is used.

Similar to `ssh` command, you can use `vnc` command to open Screen Sharing into a remote VM:

```shell
orchard vnc vm --username=administrator --password=password101 ventura-base
```

From architecture perspective, Orchard has a lower level API for port forwarding that `ssh` and `vnc` commands are built on top of.
All port forwarding connections are done via the Orchard Controller instance which "proxies" a secure connection to the Orchard Workers.
Therefore, your workers can be located under a stricter firewall that only allows connections to the Orchard Controller instance.
Orchard Controller instance is secured by default and all API calls are authenticated and authorized.
