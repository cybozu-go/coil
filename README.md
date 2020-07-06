[![GitHub release](https://img.shields.io/github/release/cybozu-go/coil.svg?maxAge=60)][releases]
[![CircleCI](https://circleci.com/gh/cybozu-go/coil.svg?style=svg)](https://circleci.com/gh/cybozu-go/coil)
[![GoDoc](https://godoc.org/github.com/cybozu-go/coil?status.svg)][godoc]
[![Go Report Card](https://goreportcard.com/badge/github.com/cybozu-go/coil)](https://goreportcard.com/report/github.com/cybozu-go/coil)

Coil
====

**Coil** is a [CNI][]-based network plugin for Kubernetes.

Coil is designed with respect to the UNIX philosophy.  It is not tightly
integrated with routing daemons like [BIRD][].  It does not implement
[Kubernetes Network Policies][NetworkPolicy] either.

Instead, you can use Coil with any routing software and policy
implementation of your choice.

Status
------

Version 2 is under **active development**.

Coil version 1 is in [release-1.1](https://github.com/cybozu-go/coil/tree/release-1.1) branch.

Requirements
------------

- Linux with routing software such as [BIRD][].
- Kubernetes Version
    - 1.18
    - Other versions are likely to work, but not tested.

Features
--------

Refer to [the design document](./docs/design.md) for more information on these features.

- Address pools

    Coil can have multiple pools of IP addresses for different purposes.
    For instance, you may have a pool of global IP addresses for Internet-facing pods
    and a pool of private IP addresses for the other pods.

    If you run out of addresses, you can add additional addresses to the pool.

- Running with any routing software

    Each node is assigned blocks of addresses, and these addresses need to be
    advertised by some routing software to enable inter-node communication.

    Coil exports these addresses to an unused Linux kernel routing table.
    Routing software such as [BIRD][] can import routes from the table and
    advertise them.

- On-demand NAT for egress traffics

    Coil can implement SNAT _on_ Kubernetes.  You can define SNAT routers
    for external networks as many as you want.

    Only selected pods can communicate with external networks via SNAT
    routers.

Examples
--------

A real world usage example of Coil can be found in [Project Neco](https://blog.kintone.io/entry/neco).
The project uses Coil with:

- [BIRD][] to advertise routes over BGP,
- [MetalLB][] to implement [LoadBalancer] Service, and
- [Calico][] to implement [NetworkPolicy][].

Programs
--------

This repository contains these programs:

- `coil`: [CNI][] plugin.
- `coild`: A background service to manage IP address.
- `coil-installer`: installs `coil` and CNI configuration file.
- `coil-controller`: watches kubernetes resources for coil.
- `coil-egress`: controls SNAT router pods.
* `hypercoil`: all-in-one binary.

Install
-------

TBD

Documentation
-------------

[docs](docs/) directory contains documents about designs and specifications.

[mtest/bird.conf](mtest/bird.conf) is an example configuration for [BIRD][] to make it work with coil.

Docker images
-------------

The official Docker image is on [Quay.io](https://quay.io/repository/cybozu/coil)

License
-------

MIT

[releases]: https://github.com/cybozu-go/coil/releases
[godoc]: https://godoc.org/github.com/cybozu-go/coil
[CNI]: https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/network-plugins/
[BIRD]: https://bird.network.cz/
[LoadBalancer]: https://kubernetes.io/docs/concepts/services-networking/service/#loadbalancer
[NetworkPolicy]: https://kubernetes.io/docs/concepts/services-networking/network-policies/
[MetalLB]: https://metallb.universe.tf
[Calico]: https://www.projectcalico.org
