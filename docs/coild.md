`coild`
=======

`coild` runs on all nodes to accept requests from `coil` CNI plugin.

Command-line flags for etcd connection are defined in [etcdutil][].

Other command-line options are:

Option           | Default value    | Description
------           | ---------------- | -----------
`http`           | "127.0.0.1:9383" | REST API endpoint.
`table-id`       | 119              | Routing table ID to export routes
`protocol-id`    | 30               | Route author ID

Etcd Endpoints lookup
---------------------

`coild` looks for [`Endpoints`][Endpoints] resource in `kube-system` namespace
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

Environment variables
---------------------

`coild` requires following environment variables to be set.

* `COIL_NODE_NAME`: The node name where `coild` is running.
* `COIL_NODE_IP`: A routable IP address to the node.

As `coild` should run as [`DaemonSet`](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/), these environment variables can be given as follows:

```yaml
      containers:
        - name: coild
          image: quay.io/cybozu/coil:0
          command:
            - /coild
          env:
            - name: COIL_NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
            - name: COIL_NODE_IP
              valueFrom:
                fieldRef:
                  fieldPath: status.hostIP
```

[etcdutil]: https://github.com/cybozu-go/etcdutil
[Endpoints]: https://kubernetes.io/docs/concepts/services-networking/service/#services-without-selectors
