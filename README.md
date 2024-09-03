# Orchard

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
