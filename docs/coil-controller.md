`coil-controller`
=================

Option           | Default value  | Description
------           | -------------  | -----------
`etcd-endpoints` |                | comma-separated URLs of the backend etcd
`etcd-password`  | ""             | password for etcd authentication
`etcd-prefix`    | `/coil/`       | etcd prefix
`etcd-timeout`   | `2s`           | dial timeout to etcd
`etcd-tls-ca`    | ""             | Path to CA bundle used to verify certificates of etcd endpoints.
`etcd-tls-cert`  | ""             | Path to my certificate used to identify myself to etcd servers.
`etcd-tls-key`   | ""             | Path to my key used to identify myself to etcd servers.
`etcd-username`  | ""             | username for etcd authentication
