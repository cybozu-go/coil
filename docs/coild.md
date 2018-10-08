`coild`
=======

`coild` runs on all nodes to accept requests from `coil` CNI plugin.

It has the following command-line options:

Option           | Default value                                 | Description
------           | -------------                                 | -----------
`etcd-endpoints` | `http://127.0.0.1:2379,http://127.0.0.1:4001` | comma-separated URLs of the backend etcd
`etcd-password`  | ""                                            | password for etcd authentication
`etcd-prefix`    | `/coil/`                                      | etcd prefix
`etcd-timeout`   | `2s`                                          | dial timeout to etcd
`etcd-tls-ca`    | ""                                            | Path to CA bundle used to verify certificates of etcd endpoints.
`etcd-tls-cert`  | ""                                            | Path to my certificate used to identify myself to etcd servers.
`etcd-tls-key`   | ""                                            | Path to my key used to identify myself to etcd servers.
`etcd-username`  | ""                                            | username for etcd authentication
`http`           | "127.0.0.1:9383"                              | REST API endpoint.
`table-id`       | 119                                           | Routing table ID to export routes
