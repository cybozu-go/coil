`coil-controller`
=================

`coil-controller` watches Kubernetes Nodes to house-keep coil resources.

Command-line flags for etcd connection are defined in [etcdutil][].

Other command-line options are:

| Option               | Default value | Description                               |
| ------               | ------------- | -----------                               |
| `scan-interval`      | 10m           | Scan interval of IP address inconsistency |
| `address-expiration` | 24h           | Expiration for alerting unused address    |

Etcd Endpoints lookup
---------------------

`coil-controller` looks for [`Endpoints`][Endpoints] resource in `kube-system` namespace
if `-etcd-endpoints` option value begins with `@`.

If the value is `@myetcd`, it looks for `kube-system/myetcd` Endpoints and
connect etcd servers using IP addresses listed in the resource.

Such a Endpoints can be created with YAML like this:
```yaml
kind: Endpoints
apiVersion: v1
metadata:
  name: myetcd
subsets:
  - addresses:
      - ip: 1.2.3.4
      - ip: 5.6.7.8
    ports:
      - port: 2379
```

[etcdutil]: https://github.com/cybozu-go/etcdutil
[Endpoints]: https://kubernetes.io/docs/concepts/services-networking/service/#services-without-selectors
