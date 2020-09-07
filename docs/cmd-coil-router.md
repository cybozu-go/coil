coil-router
===========

`coil-router` is an _optional_ program to setup the kernel routing table
to route Pod packets between Nodes.  `coil-router` can be used only when
all the nodes are in a flat layer-2 network.

## How it works

Coil allocates address blocks to Nodes.  Therefore, each node should receive
packets to the addresses in the address blocks it owns.

`coil-router` retrieves address block allocation information and configures
the kernel routing table so that each address block are routed to their
owning node.

This behavior assumes that all the nodes are directly connected in a flat
layer-2 network.

## Environment variables

`coil-router` references the following environment variables:

| Name             | Required | Description                              |
| ---------------- | -------- | ---------------------------------------- |
| `COIL_NODE_NAME` | YES      | Kubernetes node name of the running node |

## Command-line flags

**CAVEAT**: `--protocol-id` value must be different from the value of `coild`.

```
Flags:
      --health-addr string         bind address of health/readiness probes (default ":9389")
  -h, --help                       help for coil-router
      --metrics-addr string        bind address of metrics endpoint (default ":9388")
      --protocol-id int            route author ID (default 31)
      --update-interval duration   interval for forced route update (default 10m0s)
  -v, --version                    version for coil-router
```

## Prometheus metrics

### `coil_router_syncs_total`

This is a counter of the total number of route synchronizations.

| Label  | Description            |
| ------ | ---------------------- |
| `node` | The node resource name |

### `coil_router_routes_synced`

This is a gauge of the number of routes last synchronized to the kernel.

| Label  | Description            |
| ------ | ---------------------- |
| `node` | The node resource name |
