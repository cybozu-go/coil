coil-egress-controller
===============

`coil-egress-controller` is a Kubernetes controller for Coil custom resources related to on-demand NAT egress.
It is intended to be run as a Pod in `kube-system` namespace.


## Egress

`coil-egress-controller` creates **Deployment** and **Service** for each Egress.

It also creates `coil-egress` **ServiceAccount** in the namespace of Egress,
and binds it to the **ClusterRoles** for `coil-egress`.

## Command-line flags

```
Flags:
      --cert-dir string                 directory to locate TLS certs for webhook (default "/certs")
      --egress-port int32               UDP port number used by coil-egress (default 5555)
      --health-addr string              bind address of health/readiness probes (default ":9387")
  -h, --help                            help for coil-egress-controller
      --metrics-addr string             bind address of metrics endpoint (default ":9386")
  -v, --version                         version for coil-egress-controller
      --webhook-addr string             bind address of admission webhook (default ":9443")
      --enable-cert-rotation            enables webhook's certificate generation
      --enable-restart-on-cert-refresh  enables pod's restart on webhook certificate refresh
```
