Notes on Coil design
====================

Coil is a network management plugin for containers in the Kubernetes cluster.
Coil provides pod network connectivity over nodes with BGP routing

Coil aims to reduce routes in the routing reflector, inspired by [Romana][].
The advertise address block with network prefix instead of the pod's IP address.
Each pods can find other pods' IP addresses by advertise block, and they can communicate over nodes.

Architecture
------------

Coils is consist of two components, an CNI plugin `coil` and the daemon `coild` as shown in the following figure:

![Coil Architecture](http://www.plantuml.com/plantuml/png/LP91RnCn48Nl_XLFuPALs21Qk4HHLD9AA0Ag2iIj5sSzIQruny4URm-8_uvtrvicDtlqlVV6JxwBeeEarYcZHUzq990qHWLgHFF14VJ9HWeT1QKkfhD7RsY6lmeu2sV19x5yGBuxUkPvuXJ9m3AE59XSDOEEQBnrXLJ0c-KnxSYHA61UORFz-J2UlWtI_jmBAs2rkd_ShjUJ5TvzjuSNsRX44sIg33reQZt_8ide1Q8m1Q5EftezU2mn_rZ1SkUFXEokC5hNZlPI69EXcmhRfoy_4EXFeYW5CwYDV-N2bQTb-hQ2DWPbWBqF_JrGZDWvtnodb5KT-lNgSyod2aolEMhY2pdbb4uo-Rb24qWBeIDvUV_CVQ3cNZegnyc7snkSAx_S4gl5aBqO2_-d57iX33Fu_V0NbjTRPySOxO5ROedN-63Iunq5iP6kXUYinkRhar9ZQILvo2YZrIAT5cy_Rebxbw9Gm9PpUwqMwrdzZXIc9ig2ZUUzFQH0GqRlrOXJTyGcjUu_)
<!-- go to http://www.plantuml.com/plantuml/ and enter the above URL to edit the diagram. -->

### CNI plugin

The CNI plugin is invoked when new pod is created to assign addresses to the pod by kubelet.
The `coil` responsible to set-up new network for the pod.  It requests for new IP address of the pod to to `coild`.
After new IP address is given by `coild`, `coil` creates veth, assign IP address to container, and set it's route as `/32` network into kernel.
Note that the route set by `coil` is only used between node and pod.  The route is not advertise outside node.

### Assigning IP Addresses and address blocks

The `coild` responsible new IP address for the request from `coil`.
It is a daemon program ran on the node to listen requests from `coil`.

Address blocks assigned to the node are stored in etcd, and they are managed by `coild`.
The `coild` assigns new address block to the node if no address blocks are assigned to the node, or IP addresses is fully used in the assigned block.
The `coild` also consider Kubernetes cluster and node information by apiserver to assign address block.

### Advertising routes

Coil does not advertise the address directly.
Coil utilizes BGP routing daemon such as BIRD.
The kernel can hold additional routing tables.
The `coild` stores the routing of the address block to it.
BGP routing daemon lookups routes in that table, and advertise them to BGP network.
The pods can connect to over nodes as the nodes communicate advertised address block each other.

[CNI]: https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/network-plugins/
[Romana]: https://romana.io/
