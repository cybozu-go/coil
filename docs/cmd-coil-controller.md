coil-controller
===============

`coil-controller` is a Kubernetes controller for Coil custom resources.
It is intended to be run as a Pod in `kube-system` namespace.

## AddressPool and AddressBlock

`coil-controller` has an in-memory database of address pools and
address blocks to allocate address blocks quickly.

## BlockRequest

`coil-controller` watches newly created block requests and curve out
address blocks from the requested pool.

## Egress

`coil-controller` creates Deployment and Service for each Egress.

## Garbage collection

`coil-controller` periodically checks orphaned address blocks and deletes them.

## Command-line flags

```
Flags:
      --cert-dir string        directory to locate TLS certs for webhook (default "/certs")
      --gc-interval duration   garbage collection interval (default 1h0m0s)
      --health-addr string     bind address of health/readiness probes (default ":9387")
  -h, --help                   help for coil-controller
      --metrics-addr string    bind address of metrics endpoint (default ":9386")
  -v, --version                version for coil-controller
      --webhook-addr string    bind address of admission webhook (default ":9443")
```
