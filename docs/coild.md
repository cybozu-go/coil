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

[etcdutil]: https://github.com/cybozu-go/etcdutil
