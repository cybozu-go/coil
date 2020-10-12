coil-migrator
=============

`coil-migrator` is a helper to migrate existing Coil v1 cluster to v2.

It has two sub commands: `dump` and `replace`.

See [design.md](design.md#upgrading-from-v1) for the design and
[#119](https://github.com/cybozu-go/coil/pull/119#issuecomment-704674318) for the usage.

### dump sub command

This command does the followings:

- Remove Coil v1 resources from the cluster.
- Annotate namespaces using non-default address pools.
- Convert v1 data into v2 and dump them as YAML.

These steps are idempotent and can be run multiple times.

```
Usage:
  coil-migrator dump [flags]

Flags:
      --etcd-endpoints endpoints   comma-separated list of URLs (default http://127.0.0.1:2379)
      --etcd-password string       password for etcd authentication
      --etcd-prefix string         prefix for etcd keys (default "/coil/")
      --etcd-timeout string        dial timeout to etcd (default "2s")
      --etcd-tls-ca string         filename of etcd server TLS CA
      --etcd-tls-cert string       filename of etcd client certficate
      --etcd-tls-key string        filename of etcd client private key
      --etcd-username string       username for etcd authentication
  -h, --help                       help for dump
      --skip-uninstall             DANGER!! do not uninstall Coil v1

Global Flags:
      --kubeconfig string   Paths to a kubeconfig. Only required if out-of-cluster.
```

### replace sub command

This command finalizes the migration from v1 to v2 by deleting
all the currently running Pods and then deleting reserved blocks.

```
Usage:
  coil-migrator replace [flags]

Flags:
  -h, --help                help for replace
      --interval duration   interval before starting to remove pods on the next node (default 10s)

Global Flags:
      --kubeconfig string   Paths to a kubeconfig. Only required if out-of-cluster.
```
