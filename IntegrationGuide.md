# Integrating with Orchard

Orchard has a REST API that follows [OpenAPI specification](https://swagger.io/specification/) and is described in `api/openapi.yaml`.

You can run `orchard dev` locally and navigate to http://127.0.0.1:6120/v1/ for interactive documentation.

![](docs/orchard-api-documentation-browser.png)

## Resource management

Some resources, such as `Worker` and `VM`, have a `resource` field which is a dictionary that maps between resource names and their amounts (amount requested or amount provided, depending on the resource) and is useful for scheduling.

Well-known resources:

* `org.cirruslabs.tart-vms` — number of Tart VM slots available on the machine or requested by the VM
  * this number is `2` for workers and `1` for VMs by default
