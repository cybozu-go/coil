`coilctl`
=========

## Sub commands

### `pool create NAME SUBNET SIZE`

Create a new address pool.

NAME `default` is special.
Pools other than `default` will be used for Pods in the namespace of the same name.
`default` pool is used for other Pods.

SUBNET is an IPv4 or IPv6 subnet such as `10.11.0.0/16`.

SIZE is an exponent of 2.  For instance, if SIZE is 5,
the subnet will be divided into address blocks having 2^5 == 32 IP addresses.

`SUBNET / 2^SIZE` must be equal to or less than 16384 (= 2^14).

### `pool add-subnet NAME SUBNET`

Add a subnet to an existing address pool.

### `pool show NAME SUBNET`

Show address block usage of `SUBNET`.

### `pool list`

List all pool names and their subnets.

### `node blocks NODE`

List all address blocks assigned to a node.

### `address info ADDRESS`

Show ID of the container to which address of `ADDRESS` is assigned.

### `address free ADDRESS`

Free address of `ADDRESS` regardless of Pods statuses.

This subcommand is intended for fixing inconsistency between DB and real Pods.
When you find an address which is assigned in DB but is not used in any real Pods,
you can free it with this subcommand.

This should not be used in ordinary operations.


### `completion`

Generate bash completion rules.

```console
$ eval $(coilctl completion)
```

## Options

Options may be specified with command-line flags or configuration files.
Command-line flags take precedence over configuration files because of [`viper.Get`](https://godoc.org/github.com/spf13/viper#Get) specification.

### Command-line flags

Flags for etcd connection are defined in [cybozu-go/etcdutil](https://github.com/cybozu-go/etcdutil#command-line-flags).

Flags for logging are described in [cybozu-go/well](https://github.com/cybozu-go/well/wiki/Use-with-spf13-cobra).

Following flags are added or override the above specifications:

Flag            | Default value    | Description
--------------- | ---------------- | -----------
`--config`      | `$HOME/.coilctl` | [Viper configuration file](https://github.com/spf13/viper#reading-config-files).
`--etcd-prefix` | `/coil/`         | prefix for etcd keys.

### Config file

`coilctl` can read etcd connection parameters from a configuration file.
The configuration file format may be one of YAML, JSON, or TOML.

The syntax is defined in [cybozu-go/etcdutil](https://github.com/cybozu-go/etcdutil#yamljson-configuration-file).  Following parameters override the specification:

Name     | Type   | Default  | Description
-------- | ------ | -------- | -----------
`prefix` | string | `/coil/` | prefix for etcd keys.

Example:

```yaml
endpoints:
  - http://127.0.0.1:2379
username: coil
password: xxxxx
```

### Environment variables

`coilctl` can read configuration parameters from environment variables.

Use `COILCTL_` prefix to give parameters via environment variables.
For example, set `COILCTL_TLS_CA_FILE` variable to specify the CA file instead of `tls-ca-file` in a configuration file.

### Etcd Endpoints lookup

`coilctl` looks for [`Endpoints`][Endpoints] resource in `kube-system` namespace
if `-etcd-endpoints` option value begins with `@`.

If the value is `@myetcd`, it looks for `kube-system/myetcd` Endpoints and
connect etcd servers using IP addresses listed in the resource.
