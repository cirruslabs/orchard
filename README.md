# Orchard

> [!IMPORTANT]
>
> **macOS 15 (Sequoia)**
>
> The  [newly introduced "Local Network" permission](https://developer.apple.com/documentation/technotes/tn3179-understanding-local-network-privacy) on macOS Sequoia requires accepting a GUI pop-up on each machine that runs the Orchard Worker.
>
> To work around this, upgrade to Orchard 0.32.0 or newer and invoke the `orchard worker run` as `root` with an additional `--user` command-line argument, which takes the username of your regular non-privileged user.
>
> This will cause the Orchard Worker to start a small `orchard localnetworkhelper` process and then drop the privileges to the specified user.
>
>The helper process runs privileged and establishes network connections for the `orchard worker run` without triggering a GUI pop-up.
>
>This approach is more secure than simply running `orchard worker run` as `root`, because only a small part of Orchard Worker is privileged and the only thing it can do is establish new connections.

<img src="https://github.com/cirruslabs/orchard/raw/main/docs/OrchardSocial.png"/>

Orchard is an orchestration system for [Tart](https://github.com/cirruslabs/tart). Create a cluster of bare-metal Apple Silicon machines and manage dozens of VMs with ease!

## Usage

The fastest way to get started with Orchard is to use a local development mode:

```shell
brew install cirruslabs/cli/orchard
orchard dev
```

This will start Orchard Controller and a single Orchard Worker on your local machine.

You can interact with the newly created cluster using the `orchard` CLI or programmatically, through the built-in REST API server.

Please check out the [official documentation](https://tart.run/orchard/quick-start/) for more information and/or feel free to use [issues](https://github.com/cirruslabs/orchard/issues) for the remaining questions.
