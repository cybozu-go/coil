Notes on Coil design
====================

Coil is an IP address management (IPAM) tool that implements [CNI][] specifications.
Kubernetes clusters that enable CNI can use Coil as a network plugin.

Overview
--------

Coil consists of three components, `coil`, `coild`, and `coilctl` as shown below:

![Coil Architecture](http://www.plantuml.com/plantuml/svg/LP7DRjim48JlV8g1Up61oa6Jzb88Hk86A8osWP6WjrneSQqGfaYNN2g7ekzUwaUoVPCMCnzdTkyZgy2fiK8hLdiL2SIL5e4gLgws17KoaK9BGOYBwUB9QrhWhm1cuoBunCRLoF-MNjqokHH9mpWSAJYoSW4LKNbZHsLsdv77j2TBd6TBYSts-N7u-j5Rk-_EA86o_FQqNQyd53xyDFKRRsoYIQHb5ZqgQhnx8SxI2ud0z12AT2hMFUChbfzapjdw8o7J1GPqOUd0eqPdqQt4TlVm2u7-98eyoZHMeUEl-jLbsPr4P1_e9X07Gor1QHqeHegpfLobq-gytC6ryzss3ZuqYertKuoowFd5dEEpXAFtd6K2pu6rVtSvyB2qhFmYKGLIJ6Y9tpw-kco0SfrrS0wJcxjRp3Uvl13AiTjmaNz2E9zX_Gp-C3OUKs1lVNNCn1XDxHfo7A_bFQIJvyQScryRqHg5pVUT47tFgSnbdDpeGUAPQXEP0gQGUMFNAF4xKfW0OyZAkuEfKw3kdHvQiHNtv7Hgx7y0)
<!-- go to http://www.plantuml.com/plantuml/ and enter the above URL to edit the diagram. -->

* `coil` is the CNI plugin.  It is installed in `/opt/cni/bin` directory.
* `coild` is a background service that communicates with Kubernetes API server and etcd.
* `coilctl` is a command-line tool to set or get Coil configurations and statuses.

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
