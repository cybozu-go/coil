coild
=====

`coild` is a gRPC server running on each node.

## gRPC server

`coild` listens on a UNIX domain socket and accepts requests from `coil`
over gRPC protocol.  The default socket path is `/run/coild.sock`.

The gRPC server provides following additional features:

- [gRPC Server Reflection](https://github.com/grpc/grpc-go/blob/master/Documentation/server-reflection-tutorial.md)
- [gRPC metrics](https://github.com/grpc-ecosystem/go-grpc-prometheus#metrics)
- Access logging

## Pod routes

`coild` registers the routes to local Pods into a kernel routing table.
The default routing table ID is **116**.

This routing table is looked up by a routing rule inserted by `coild`.
The default rule priority is **2000**.

## Route export

`coild` exports address blocks owned by the running node to a kernel
routing table.  The default routing table ID is **119**.

The routes are created in that table with a specific author (protocol) ID.
The default protocol ID is **30**.

## Compatibility with Calico

`coild` optionally can make veth interface names compatible with Calico.
If you want to use Calico for network policy together with Coil, enable
this feature with `--compat-calico` flag.

Calico needs to be configured to set [`FELIX_INTERFACEPREFIX`](https://github.com/projectcalico/calico/blob/c0fe9f811ea8721007df9362d63af6697b42f6f3/reference/felix/configuration.md#bare-metal-specific-configuration) to `veth`.

## Environment variables

`coild` references the following environment variables:

| Name             | Required | Description                              |
| ---------------- | -------- | ---------------------------------------- |
| `COIL_NODE_NAME` | YES      | Kubernetes node name of the running node |

## Command-line flags

```
Flags:
      --backend string        backend for egress NAT rules: iptables or nftables (default: iptables)
      --compat-calico         make veth name compatible with Calico
      --egress-port int       UDP port number for egress NAT (default 5555)
      --enable-egress         enable Egress related features (default true)
      --enable-ipam           enable IPAM related features (default true)
      --export-table-id int   routing table ID to which coild exports routes (default 119)
      --health-addr string    bind address of health/readiness probes (default ":9385")
  -h, --help                  help for coild
      --metrics-addr string   bind address of metrics endpoint (default ":9384")
      --pod-rule-prio int     priority with which the rule for Pod table is inserted (default 2000)
      --pod-table-id int      routing table ID to which coild registers routes for Pods (default 116)
      --protocol-id int       route author ID (default 30)
      --register-from-main    help migration from Coil 2.0.1
      --socket string         UNIX domain socket path (default "/run/coild.sock")
  -v, --version               version for coild
```
