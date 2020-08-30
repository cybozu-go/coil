User manual
===========

- [Defining address pools](#defining-address-pools)
  - [Default pool](#default-pool)
  - [Other pools](#other-pools)
  - [Adding addresses to a pool](#adding-addresses-to-a-pool)

## Defining address pools

### Default pool

The first thing you need to do after installation is to define the default address pool.
The default pool is an `AddressPool` resource whose name is `default`.

```yaml
apiVersion: coil.cybozu.com/v2
kind: AddressPool
metadata:
  name: default
spec:
  blockSizeBits: 5
  subnets:
    - ipv4: 10.2.0.0/16
      ipv6: fd01:0203:0405:0607::/112
```

The above example defines a dual-stack pool.
You can define IPv4 pool by removing `ipv6` field.  Likewise, to define IPv6 pool, remove `ipv4` field.

For a dual stack pool, IPv4 subnet and IPv6 subnet must be the same size.

`blockSizeBits` defines the size of an address block curved from the pool.
If it is 5, a block of this pool will have 32 (= 2^5) addresses.
The minimum `blockSizeBits` is 0.
After creation, you cannot change `blockSizeBits`.

### Other pools

You may define other address pools.  Non-default pools are used only if Namespace has a special annotation `coil.cybozu.com/pool`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: global
  annotations:
    coil.cybozu.com/pool: global
```

Pods in `global` namespace will have addresses from `global` AddressPool.

### Adding addresses to a pool

If a pool is running out of IP addresses, you can add more subnets.

```yaml
apiVersion: coil.cybozu.com/v2
kind: AddressPool
metadata:
  name: default
spec:
  blockSizeBits: 5
  subnets:
    - ipv4: 10.2.0.0/16
      ipv6: fd01:0203:0405:0607::/112
    - ipv4: 10.3.0.0/16
      ipv6: fd01:0203:0405:0608::/112
```

Removing existing subnets is forbidden.
