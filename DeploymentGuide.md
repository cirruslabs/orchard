# Orchard Cluster Deployment Guide

Orchard cluster consists of two components: Orchard Controller and a pool of Orchard Workers. Orchard Controller is
responsible for managing the cluster and scheduling of resources. Orchard Workers are responsible for executing the VMs.

The following guide is split in two parts. First, we'll [deploy an Orchard Controller](#deploying-orchard-controller) and then we'll
[configure and register Orchard Workers](#configuring-orchard-workers) with Ansible.

## Securing the communications with an Orchard Controller

When an Orchard client or Worker connects to the Controller, they need to establish trust somehow and verify that they're talking to the right Controller and no [man-in-the-middle attack](https://en.wikipedia.org/wiki/Man-in-the-middle_attack) is possible.

Similarly to most web-browsers that rely on the PKI ([public key infrastructure](https://en.wikipedia.org/wiki/Public_key_infrastructure)), Orchard uses a hybrid approach by defaulting to automatic PKI verification (can be disabled by [`--no-pki`](#--no-pki-override)) and falling-back to a manual verification for self-signed certificates.

In this section, we assume the following Controller types:

* *Controller with a publicly valid certificate*
  * can be configured by passing `--controller-cert` and `--controller-key` command-line arguments to `orchard controller run`
* *Controller with a self-signed certificate*
  * configured automatically on first Controller start-up when no `--controller-cert` and `--controller-key` are presented

Below we'll explain how Orchard client and Worker handle these two Controller types.

### Client

Client connects (or in other words, associates) with the Controller using a `orchard context create` command.

The default mode allows for connecting to both Controller types (see above) by leveraging a fall-back mechanism:

* Client first tries to connect to the Controller and validates its certificate using host's root CA set (can be disabled with [`--no-pki`](#--no-pki-override))
* if the Client has encountered a *Controller with a publicly valid certificate*, that would be the last step and the association would succeed
* if the Client is dealing with *Controller with a self-signed certificate*, the Client will do another connection attempt to probe the Controller's certificate
* the probed Controller's certificate fingerprint is then presented to the user, and if the user agrees to trust it, the Client then considers that certificate to be trusted for a given Controller address
* Client finally connects to the Controller again with a trusted CA set containing only that certificate, execute the final API sanity checks and if everything is OK then the association succeeds

Afterward, each interaction with the Controller  (e.g. `orchard create vm` command) will stick to the chosen verification method and will re-verify the presented Controller's certificate against:

* PKI association: host's root CA set
* non-PKI association: a trusted certificate stored in the Orchard's configuration file

Another thing to note is that PKI and non-PKI associations will emit slightly different Boostrap Tokens for use in Worker:

* PKI: Bootstrap Token won't include the Controller's certificate, thus forcing the Worker to validate the Controller's certificate against the Worker's host root CA set
* non-PKI: Bootstrap Token will include the Controller's certificate, thus forcing the Worker to validate the Controller's certificate only against that Controller's certificate

### Worker

Worker connects to the Controller using a `orchard worker run` command.

The default mode allows for connecting to both Controller types (see above) by looking at the Bootstrap Token contents:

* if the Bootstrap Token was generated on a Client that associated with a Controller with the help of the PKI
  * the bootstrap token would contain no Controller certificate
  * the Orchard Worker will try the PKI approach (can be disabled with [`--no-pki`](#--no-pki-override) to effectively prevent the Worker from connecting) and fail if certificate verification using PKI is possible
* if the Bootstrap Token was generated on a Client that associated with a Controller by utilizing a manual fingerprint verification
  * the bootstrap token would contain a Controller certificate
  * the Orchard Worker will try to connect to the Controller with a trusted CA set containing only that certificate

#### `--no-pki` override

If you're accessing a *Controller with a self-signed certificate* and want to additionally guard yourself against [CA compromises](https://en.wikipedia.org/wiki/Certificate_authority#CA_compromise) and other PKI-specific attacks, pass a `--no-pki` command-line argument to the following commands:

* `orchard context create --no-pki`
  * this will prevent the Client from using PKI and will let you interactively verify the Controller's certificate fingerprint before connecting
* `orchard worker run --no-pki`
  * this will prevent the Worker from trying to use PKI when connecting to the Controller using a Bootstrap Token that has no certificate included in it

Note that we've deliberately chosen not to use environment variables (e.g. `ORCHARD_NO_PKI`), because it's hard to be sure whether the variable was picked up by the command or not.

Compared to environment variables, an invalid command-line argument will result in an error that wouldn't let you run the command.

## Deploying Orchard Controller

Orchard API is secured by default: all requests must be authenticated with credentials of a service account.
When you first run Orchard Controller, you can specify `ORCHARD_BOOTSTRAP_ADMIN_TOKEN` which will automatically
create a service account named `bootstrap-admin` with all privileges. Let's first generate `ORCHARD_BOOTSTRAP_ADMIN_TOKEN`:

```bash
export ORCHARD_BOOTSTRAP_ADMIN_TOKEN=$(openssl rand -hex 32)
```

Now you can run Orchard Controller on a server of your choice. In the following sections you'll find several examples of
how to run Orchard Controller in various environments. Feel free to submit PRs with more examples.

### Google Cloud Compute Engine

An example below will deploy a single instance of Orchard Controller in Google Cloud Compute Engine in `us-central1` region.

First, let's create a static IP address for our instance

```bash
gcloud compute addresses create orchard-ip --region=us-central1
export ORCHARD_IP=$(gcloud compute addresses describe orchard-ip --format='value(address)' --region=us-central1)
```

Once we have the IP address, we can create a new instance with Orchard Controller running inside a container:

```bash
gcloud compute instances create-with-container orchard-controller \
  --machine-type=e2-micro \
  --zone=us-central1-a \
  --image-family cos-stable \
  --image-project cos-cloud \
  --tags=https-server \
  --address=$ORCHARD_IP \
  --container-image=ghcr.io/cirruslabs/orchard:latest \
  --container-env=PORT=443 \
  --container-env=ORCHARD_BOOTSTRAP_ADMIN_TOKEN=$ORCHARD_BOOTSTRAP_ADMIN_TOKEN \
  --container-mount-host-path=host-path=/home/orchard-data,mode=rw,mount-path=/data
```

Now you can create a new context for your local client:

```bash
orchard context create --name production \
  --service-account-name bootstrap-admin \
  --service-account-token $ORCHARD_BOOTSTRAP_ADMIN_TOKEN \
  https://$ORCHARD_IP:443
```

And select it as the default context:

```bash
orchard context default production
```

## Configuring Orchard Workers

First, create a service account limited with a minimal set of roles required for proper worker functioning:

```bash
orchard create service-account worker-pool-m1 --roles "compute:read" --roles "compute:write"
```

Then, generate a bootstrap token:

```shell
orchard get bootstrap-token worker-pool-m1
```

Then, for each worker machine, start the worker as follows:

```shell
orchard worker run --bootstrap-token <bootstrap token from the step above> <controller URL>
```

### Automation

If you have a set of machines that you want to use as Orchard Workers, you can use Ansible to configure them.
Please refer a [separate repository](https://github.com/cirruslabs/ansible-orchard) where we prepared a basic
Ansible playbook for convenient setup.

## Observability

Both the controller and worker produce some useful OpenTelemetry metrics. Metrics are scoped with `org.cirruslabs.orchard` prefix and include information about resource utilization, statuses or workers, scheduling/pull time and many more.

By default, the telemetry is sent to https://localhost:4317 using the gRPC protocol and to http://localhost:4318 using the HTTP protocol.

You can override this by setting the [standard OpenTelemetry environment variable](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/) `OTEL_EXPORTER_OTLP_ENDPOINT`.

Please refer to [OTEL Collector documentation](https://opentelemetry.io/docs/collector/) for instruction on how to setup a sidecar for the metrics collections or find out if your SaaS monitoring has an available OTEL endpoint (see [Honeycomb](https://docs.honeycomb.io/send-data/opentelemetry/) as an example).

### Sending metrics to Google Cloud Platform

There are two standard options of ingesting metrics procuded by Orchard controller and workers into the GCP:

* [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/) + [Google Cloud Exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/exporter/googlecloudexporter/README.md) — open-source solution that can be later re-purposed to send metrics to any OTLP-compatible endpoint by swapping a single [exporter](https://opentelemetry.io/docs/collector/configuration/#exporters)
* [Ops Agent](https://cloud.google.com/monitoring/agent/ops-agent/otlp) — Google-backed solution with a syntax similar to OpenTelemetry Collector, but tied to GCP-only
