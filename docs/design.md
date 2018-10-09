Notes on Coil design
====================

Coil is an IP address management (IPAM) tool that implements [CNI][] specifications.
Kubernetes clusters that enable CNI can use Coil as a network plugin.

Overview
--------

Coil consists of three components, `coil`, `coild`, and `coilctl` as shown below:

![Coil Architecture](http://www.plantuml.com/plantuml/svg/RL9DRnen5BpxLupe1KjTeYRj9K8eeP4gr3P2gNhBnQmVYuMngVqCXwh_lJQBMVfniv_cOppF3v5LuBIpGYjMAnK9nAqMWLfMhhC4jJUHme626BVUsARrZeEtbeA4YXYkOZYYqVxecjb0liVOI1mLjniuhod-3rsS2p0ZMqlAGMaTA4Qih6-tsIsQpkt8SHJYAk7eEGkEy5C0bqUCWaziN8Tyj_JgnicbI1h6OKl1aPMZFhsnPwG01ibjMf4bphEnn7pnyjIVFf-evLo84fXEFpulPhCgXJTVJBVXkKrKGLfcq9EYdNVVY3Fq2Y9GZT2aVIwW4781xsJEUV1RGbONwAKRIsi-OqfBjnABvUDN1FgHAF8PfZ59-qbjyauYiWzq4uY3eGFLHYbUyDD261RLivQ-LBNsOQVOU5SpJ9jGmZUN4Eyb71rpa2fSaNMbVMCP-K6Y3QIOS23Ul7rrcG3b2hLdzfERkriC2xbQJyvvyfxqw_WbXFDCECtWrwVfVKwHdy0cqbzVvs0Kvf-MGlHlhkckz7F4HuaNwana2gYkkO8_fSJt-C-FRRrRcou5ElaKqPQjU22dqyx-1W00)
<!-- go to http://www.plantuml.com/plantuml/ and enter the above URL to edit the diagram. -->

* `coil` is the CNI plugin.  It is installed in `/opt/cni/bin` directory.
* `coild` is a background service that communicates with Kubernetes API server and etcd.
* `coilctl` is a command-line tool to set or get Coil configurations and statuses.
* `coil-controller` watches API server for house-keeping.

### How it works

1. `coil` is invoked by `kubelet` when a new pod is created.
2. `coil` communicates with `coild` to obtain a new IP address for the Pod.
3. `coild` queries Pod information to API server, and IPAM configurations to etcd.
4. `coild` assigns a new IP address for the Pod and returns it to `coil`.
5. `coil` configures network links (veth) and in-host routing for the Pod.

Address block
-------------

In order to reduce the number of routes in the system, Coil divides a large
subnet into small fixed-size blocks called _address block__.  For example,
if coil is configured to allocate IP addresses from `10.1.0.0/16`, it may
divides the subnet into 4096 blocks of the size of `/28` (16 addresses).

This is inspired by [Romana][].

Each node has one or more address blocks.  `coild` is responsible for managing
address blocks.  `coil` only receives a single IP address (not a subnet).

House-keeping
-------------

Coil need to reclaim IP addresses and address blocks when they are no longer used.

When a node is removed from Kubernetes cluster, `coil-controller` will reclaim
IP addresses and address blocks kept for that node.

When a Pod is removed, `coild` will free the IP address for the Pod.  It will *not*
return address blocks to the pool at the same time to avoid complex concurrency
control.

When `coild` starts or restarts, it will first examine which address blocks and
IP addresses are kept for the node by examining etcd database.  It then queries
the API server to obtain the currently running Pods, and reclaim IP addresses
kept for missing Pods.  If no IP addresses are kept for an address block, `coild`
will return it to the pool.  Once `coild` completes this house-keeping, it starts
accepting REST API requests.

Routing
-------

`coil` configures routing rules within a node.

Unlike existing CNI plugins such as [Calico][] or [Romana][], Coil does not
configure routes between nodes by itself.  Instead, it exports routing
information to an unused kernel routing table to allow other programs
like [BIRD][] can import them and advertise via BGP, RIP, or OSPF.

[CNI]: https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/network-plugins/
[Calico]: https://www.projectcalico.org/
[Romana]: https://romana.io/
[BIRD]: http://bird.network.cz/
