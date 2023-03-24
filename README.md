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

TBD
