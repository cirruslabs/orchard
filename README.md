# Orchard

Orchard is an orchestration system for [Tart](https://github.com/cirruslabs/tart).

Create a cluster of bare-metal Apple Silicon machines and manage dozens of VMs with ease!

## Installation

```
go install github.com/cirruslabs/orchard/...@latest
```

## Quick start

Start the Orchard Controller and the Worker in a single inocation:

```shell
orchard dev
```

Create a Virtual Machine resource:

```shell
orchard create vm --image ghcr.io/cirruslabs/macos-ventura-base:latest ventura-base
```

Check a list of VM resources to see if the Virtual Machine we've created above is already running: 

```shell
orchard list vms
```

## Development

Development is done as one would normally develop any Golang package, however, if you did modify any `*.proto` files in the `rpc/` directory, install [Buf](https://buf.build/) and run the following command from the project's root directory to re-generate the code:

```shell
buf generate
```
