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

## In-cluster networks for egress NAT

For each Pod using egress NAT, `coild` (and `coil-egress-controller`'s
per-node `EgressWatcher`) install `ip rule`s that route traffic to
in-cluster destinations through the normal (non-NAT) path, bypassing the
FoU tunnel even if an `Egress` resource's `spec.destinations` would
otherwise match them (e.g. a wide destination such as `0.0.0.0/0` or
`::/0`). By default, "in-cluster" means RFC1918 for IPv4
(`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`) and `fc00::/7` (ULA) for
IPv6.

If your cluster uses globally routable IPv6 addresses (or IPv4 ranges
outside RFC1918) for Pods, Services, or Nodes, the IPv6 (or IPv4) default
above does not cover them, so an `Egress` with a wide destination like
`::/0` would route all Pod/Service/Node traffic through the NAT tunnel.
Use `--cluster-networks` to override the default in-cluster networks with
your cluster's actual Pod/Service/Node CIDRs.

The override applies per IP family: if `--cluster-networks` contains at
least one network of a given family, it replaces the default for that
family only, while the other family keeps its built-in default. For
example, `--cluster-networks=2001:db8:1234::/48` overrides only the IPv6
default; IPv4 keeps using RFC1918.

Each in-cluster network consumes one `ip rule` priority, so at most 100
networks per IP family are supported.

Changing `--cluster-networks` on an already-running `coild` requires
restarting NAT client Pods, since existing Pods keep the rules that were
installed when their network was set up (see
[the design document](design.md#nat-configuration-updates) for the same
caveat that already applies to other NAT settings such as the FoU port).

## Environment variables

`coild` references the following environment variables:

| Name             | Required | Description                              |
| ---------------- | -------- | ---------------------------------------- |
| `COIL_NODE_NAME` | YES      | Kubernetes node name of the running node |

## Command-line flags

```
Flags:
      --backend string          backend for egress NAT rules: iptables or nftables (default: iptables)
      --cluster-networks strings CIDR networks that are excluded from egress NAT because they are routed within the cluster (default: RFC1918 for IPv4, fc00::/7 for IPv6); overrides the default per IP family when at least one network of that family is given
      --compat-calico           make veth name compatible with Calico
      --egress-port int         UDP port number for egress NAT (default 5555)
      --enable-egress           enable Egress related features (default true)
      --enable-ipam             enable IPAM related features (default true)
      --enable-originating-only egress should be used only for connections originating in the pod (default: false)
      --export-table-id int     routing table ID to which coild exports routes (default 119)
      --health-addr string      bind address of health/readiness probes (default ":9385")
  -h, --help                    help for coild
      --metrics-addr string     bind address of metrics endpoint (default ":9384")
      --pod-rule-prio int       priority with which the rule for Pod table is inserted (default 2000)
      --pod-table-id int        routing table ID to which coild registers routes for Pods (default 116)
      --protocol-id int         route author ID (default 30)
      --register-from-main      help migration from Coil 2.0.1
      --socket string           UNIX domain socket path (default "/run/coild.sock")
  -v, --version                 version for coild
```
