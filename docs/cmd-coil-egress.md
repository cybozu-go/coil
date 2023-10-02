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
