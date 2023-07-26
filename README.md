# Orchard

<img src="https://github.com/cirruslabs/orchard/raw/main/docs/OrchardSocial.png"/>

Orchard is an orchestration system for [Tart](https://github.com/cirruslabs/tart). Create a cluster of bare-metal Apple Silicon machines and manage dozens of VMs with ease!

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

#### SSH

To SSH into a VM use the `orchard ssh` command:

```shell
orchard ssh vm ventura-base
```

You can specify the `--username` and `--password` flags to specify the username/password pair to SSH. By default, `admin`/`admin` is used.

You can also execute remote commands instead of spawning a login shell, similarly to the OpenSSH's `ssh` command:

```shell
orchard ssh vm ventura-base "uname -a"
```

You can execute scripts remotely this way, by telling the remote command-line interpreter to read from the standard input and using the redirection operator as follows:

```shell
orchard ssh vm ventura-base "bash -s" < script.sh
```

#### VNC

Similar to `ssh` command, you can use `vnc` command to open Screen Sharing into a remote VM:

```shell
orchard vnc vm --username=administrator --password=password101 ventura-base
```

From architecture perspective, Orchard has a lower level API for port forwarding that `ssh` and `vnc` commands are built on top of.
All port forwarding connections are done via the Orchard Controller instance which "proxies" a secure connection to the Orchard Workers.
Therefore, your workers can be located under a stricter firewall that only allows connections to the Orchard Controller instance.
Orchard Controller instance is secured by default and all API calls are authenticated and authorized.

### Environment variables

In addition to controlling the Orchard via the CLI arguments, there are environment variables that may be beneficial both when automating Orchard and in daily use:

| Variable name                   | Description                                                                                                                                                                                                                                                                                                                                                                                                  |
|---------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `ORCHARD_HOME`                  | Override Orchard's home directory. Useful when running multiple Orchard instances on the same host and when testing.                                                                                                                                                                                                                                                                                         |
| `ORCHARD_LICENSE_TIER`          | The default license limit only allows connecting 4 Orchard Workers to the Orchard Controller. If you've purchased a [Gold Tier License](https://tart.run/licensing/), set this variable to `gold` to increase the limit to 20 Orchard Workers. And if you've purchased a [Platinum Tier License](https://tart.run/licensing/), set this variable to `platinum` to increase the limit to 200 Orchard Workers. |
| `ORCHARD_SERVICE_ACCOUNT_NAME`  | Override service account name (used for controller API auth) on per-command basis                                                                                                                                                                                                                                                                                                                            |
| `ORCHARD_SERVICE_ACCOUNT_TOKEN` | Override service account token (used for controller API auth) on per-command basis                                                                                                                                                                                                                                                                                                                           |
| `ORCHARD_URL`                   | Override controller URL on per-command basis                                                                                                                                                                                                                                                                                                                                                                 |
