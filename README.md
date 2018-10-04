[![GitHub release](https://img.shields.io/github/release/cybozu-go/coil.svg?maxAge=60)][releases]
[![CircleCI](https://circleci.com/gh/cybozu-go/coil.svg?style=svg)](https://circleci.com/gh/cybozu-go/coil)
[![GoDoc](https://godoc.org/github.com/cybozu-go/coil?status.svg)][godoc]
[![Go Report Card](https://goreportcard.com/badge/github.com/cybozu-go/coil)](https://goreportcard.com/report/github.com/cybozu-go/coil)

Coil
====

**Coil** is a [CNI][] plugin that automates IP address management (IPAM).

Coil is designed in favor of UNIX philosophy.  It is not tightly integrated
with routing daemons like [BIRD][].  It does not implement
[Kubernetes Network Policies][NetworkPolicy] by itself.

Instead, users can choose their favorite routing daemons and/or network
policy implementations.

**Project Status**: Initial development.

Requirements
------------

* [etcd][]

Planned Features
----------------

* CNI IPAM implementation

* Address block

    Coil can divide a large subnet into small fixed size blocks (e.g. `/27`),
    and assign them to nodes.  Nodes then allocate IP addresses to Pods
    from the assigned blocks.

* Publish routes via unused kernel routing table

    Coil exports address blocks assigned to the node to an unused kernel
    routing table.  [BIRD][] can be configured to look for the table and
    publish the registered routes over BGP or other protocols.

Programs
--------

This repository contains these programs:

* `coil`: [CNI][] plugin.
* `coilctl`: CLI tool to configure coil IPAM.
* `coild`: A background service to manage IP address.

`coil` should be installed in `/opt/cni/bin` directory.

`coild` should run as [`DaemonSet`](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/) container.

`coilctl` directly communicates with etcd.
Therefore it can be installed any host that can connect to etcd cluster.

Documentation
-------------

[docs](docs/) directory contains documents about designs and specifications.

License
-------

MIT

[releases]: https://github.com/cybozu-go/coil/releases
[godoc]: https://godoc.org/github.com/cybozu-go/coil
[CNI]: https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/network-plugins/
[BIRD]: https://bird.network.cz/
[NetworkPolicy]: https://kubernetes.io/docs/concepts/services-networking/network-policies/
[etcd]: https://github.com/etcd-io/etcd
