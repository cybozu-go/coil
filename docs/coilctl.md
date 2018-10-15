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

### `node blocks`

List all address blocks assigned to a node.

## Options

Option      | Default value         | Description
------      | -------------         | -----------
`config`    | `$HOME/.coilctl.yaml` | Location of the config file.

## Config file

`coilctl` can read configurations from a YAML specified in `-config-file` option.

The config file format is specified in [cybozu-go/etcdutil](https://github.com/cybozu-go/etcdutil), and not shown below will use default values of the etcdutil.

Name     | Type   | Required | Description
-------- | ------ | -------- | -----------
`prefix` | string | No       | Key prefix of etcd objects.  Default is `/coil/`.

### Example

```yaml
endpoints:
  - http://127.0.0.1:2379
username: coil
password: xxxxx
```
