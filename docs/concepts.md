Concepts
========

## Subnet

Subnet is a IPv4 or IPv6 subnet.

## Address block

Address block is a chunk of IP addresses in a subnet.
Its size may be specified with a power of 2.

Address blocks are assigned to nodes in Kubernetes cluster.

Each node will request assignment of a new address block when the
node have no more assignable IP addresses to Pods.

## Address pool

Address pool is a collection of subnets.

An address pool has following properties:

* Name

    The name of this pool.  This must be unique.
    `default` pool must be created.

    Addresses in the `default` pool may be allocated to Pods in any namespace.

    Addresses in other pools can only be allocated to Pods in the namespace of the same name.

* Address block size

    The size of address blocks from this pool.
    The number will be interpreted as an exponent of 2.

## Routing

Routing to Pods are separated in these two:

1. Intra-node routing
2. Inter-node routing

Intra-node routing is automatically programmed by `coil` using Linux veth.

Inter-node routing need to be done by users.

## Route export

In order to implement inter-node routing, address blocks
assigned to the node need to be exposed to routing daemons such as [BIRD][].

Coil exports address blocks of the node to an unused kernel routing table.
Routing daemons can learn routes from this routing table.

Note that Linux has multiple routing tables (c.f. http://linux-ip.net/html/routing-tables.html).

[BIRD]: http://bird.network.cz
