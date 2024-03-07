coil-controller
===============

`coil-controller` is a Kubernetes controller for Coil custom resources.
It is intended to be run as a Pod in `kube-system` namespace.

## AddressPool and AddressBlock

`coil-controller` has an in-memory database of address pools and
address blocks to allocate address blocks quickly.

## BlockRequest

`coil-controller` watches newly created block requests and carve out
address blocks from the requested pool.

## Egress

`coil-controller` creates **Deployment** and **Service** for each Egress.

It also creates `coil-egress` **ServiceAccount** in the namespace of Egress,
and binds it to the **ClusterRoles** for `coil-egress`.

## Garbage collection

`coil-controller` periodically checks orphaned address blocks and deletes them.

## Command-line flags

```
Flags:
      --cert-dir string        directory to locate TLS certs for webhook (default "/certs")
      --egress-port int32      UDP port number used by coil-egress (default 5555)
      --gc-interval duration   garbage collection interval (default 1h0m0s)
      --health-addr string     bind address of health/readiness probes (default ":9387")
  -h, --help                   help for coil-controller
      --metrics-addr string    bind address of metrics endpoint (default ":9386")
  -v, --version                version for coil-controller
      --webhook-addr string    bind address of admission webhook (default ":9443")
```

## Prometheus metrics

### `coil_controller_max_blocks`

This is a gauge of the maximum number of allocatable address blocks of a pool.

| Label  | Description   |
| ------ | ------------- |
| `pool` | The pool name |

### `coil_controller_allocated_blocks`

This is a gauge of the number of currently allocated address blocks.

| Label  | Description   |
| ------ | ------------- |
| `pool` | The pool name |
