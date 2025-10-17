coil-egress
===========

`coil-egress` is a program to be run in Egress pod.

It watches client Pods and creates or deletes Foo-over-UDP tunnels.

## Environment variables

`coil-egress` references the following environment variables:

| Name                 | Required | Description                                            |
| -------------------- | -------- | ------------------------------------------------------ |
| `COIL_POD_ADDRESSES` | YES      | `status.podIPs` field value of the Pod.                |
| `COIL_POD_NAMESPACE` | YES      | `metadata.namespace` field value of the parent Egress. |
| `COIL_EGRESS_NAME`   | YES      | `metadata.name` field value of the parent Egress.      |

## Command-line flags

```
Flags:
      --backend string        backend for egress NAT rules: iptables or nftables (default "iptables")
      --fou-port int          port number for foo-over-udp tunnels (default 5555)
      --enable-sport-auto     enable automatic source port assignment (default false)
      --health-addr string    bind address of health/readiness probes (default ":8081")
  -h, --help                  help for coil-egress
      --metrics-addr string   bind address of metrics endpoint (default ":8080")
  -v, --version               version for coil-egress
```

## Prometheus metrics

### `coil_egress_client_pod_count`

This is the number of client pods which use the egress.

| Label       | Description                   |
| ----------- | ----------------------------- |
| `namespace` | The egress resource namespace |
| `egress`    | The egress resource name      |

### `coil_egress_client_pod_info`

This is the client pod information.

| Label              | Description                   |
| -------------------| ----------------------------- |
| `namespace`        | The pod resource namespace |
| `pod`              | The pod name                  |
| `pod_ip`           | The pod's IP address          |
| `interface`        | The interface for the pod     |
| `egress`           | The egress resource name      |
| `egress_namespace` | The egress resource namespace |

### `coil_egress_nf_conntrack_entries_limit`

This is the limit of conntrack entries in the kernel.
This value is from `/proc/sys/net/netfilter/nf_conntrack_max`.

| Label       | Description                   |
| ----------- | ----------------------------- |
| `namespace` | The egress resource namespace |
| `egress`    | The egress resource name      |
| `pod`       | The pod name                  |


### `coil_egress_nf_conntrack_entries`

This is the number of conntrack entries in the kernel.
This value is from `/proc/sys/net/netfilter/nf_conntrack_count`.

| Label       | Description                   |
| ----------- | ----------------------------- |
| `namespace` | The egress resource namespace |
| `egress`    | The egress resource name      |
| `pod`       | The pod name                  |

### `coil_egress_nftables_masqueraded_packets_total`

This is the total number of packets masqueraded by iptables/nftables in a egress NAT pod.
This value is from the result of `iptables -t nat -L POSTROUTING -vn` or nftables equivalent.

| Label       | Description                   |
| ----------- | ----------------------------- |
| `namespace` | The egress resource namespace |
| `egress`    | The egress resource name      |
| `pod`       | The pod name                  |
| `protocol`  | The protocol name                 |

### `coil_egress_nftables_masqueraded_bytes_total`

This is the total bytes of masqueraded packets by iptables/nftables in a egress NAT pod.
This value is from the result of `iptables -t nat -L POSTROUTING -vn` or nftables equivalent.

| Label       | Description                   |
| ----------- | ----------------------------- |
| `namespace` | The egress resource namespace |
| `egress`    | The egress resource name      |
| `pod`       | The pod name                  |
| `protocol`  | The protocol name                 |

### `coil_egress_nftables_invalid_packets_total`

This is the total number of packets dropped as invalid packets by iptables/nftables in a egress NAT pod.
This value is from the result of `iptables -t filter -L -vn` or nftables equivalent.

| Label       | Description                   |
| ----------- | ----------------------------- |
| `namespace` | The egress resource namespace |
| `egress`    | The egress resource name      |
| `pod`       | The pod name                  |
| `protocol`  | The protocol name                 |

### `coil_egress_nftables_invalid_bytes_total`

This is the total bytes of packets dropped as invalid packets by iptables/nftables in a egress NAT pod.
This value is from the result of `iptables -t filter -L -vn` or nftables equivalent.

| Label       | Description                   |
| ----------- | ----------------------------- |
| `namespace` | The egress resource namespace |
| `egress`    | The egress resource name      |
| `pod`       | The pod name                  |
| `protocol`  | The protocol name                 |

