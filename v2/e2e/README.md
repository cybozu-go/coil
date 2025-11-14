End-to-end test suites
======================

This document describes the strategy, analysis, and implementation of
end-to-end (e2e) tests for Coil.

- [End-to-end test suites](#end-to-end-test-suites)
  - [Strategy](#strategy)
  - [Analysis](#analysis)
    - [Manifests](#manifests)
    - [`coil-ipam-controller`](#coil-ipam-controller)
    - [`coil-egress-controller`](#coil-egress-controller)
    - [`coild`](#coild)
    - [`coil-router`](#coil-router)
    - [`coil-egress`](#coil-egress)
  - [How to test](#how-to-test)
  - [Implementation](#implementation)

## Strategy

Almost all the functions of Coil are well-tested in unit tests and
integration tests.  The exceptions are:

- YAML manifests
- `main` function of each program
- inter-node communication
- Egress NAT over Kubernetes `Service`

Therefore, it is enough to cover these functions in e2e tests.

## Analysis

### Manifests

RBAC should carefully be examined.
The other manifests are mostly tested together with other tests.

### `coil-ipam-controller`

What the `main` function implements are:

- Leader election
- Admission webhook
- Health probe server
- Metrics server
- Reconciler for BlockRequest
- Garbage collector for orphaned AddressBlock

### `coil-egress-controller`

What the `main` function implements are:

- Leader election
- Admission webhook
- Health probe server
- Metrics server
- Reconciler for Egress

### `coild`

What the `main` function implements are:

- Health probe server
- Metrics server
- gRPC server for `coil`
- Route exporter
- Persisting IPAM status between restarts
- Setup egress NAT clients

### `coil-router`

What the `main` function implements are:

- Health probe server
- Metrics server
- Routing table setup for inter-node communication

### `coil-egress`

- Watcher for client pods

## How to test

Health probe servers can be tested by checking Pod readiness.

Reconciler for BlockRequest in `coil-ipam-controller`, gRPC server in `coild`,
and routing table setup in `coil-router` can be tested together by
checking if Pods on different nodes can communicate each other.

Admission webhook can be tested by trying to create an invalid
AddressPool that cannot be checked by OpenAPI validations.
A too narrow subnet is such an example.

Garbage collector in `coil-ipam-controller` can be tested by creating
orphaned AddressBlock manually.

Persisting IPAM status in `coild` can be tested by restarting `coild` Pods
and examine the allocation status.

Egress NAT feature can be tested by running Egress pods on a specific
node and assigning a fake global IP address such as `9.9.9.9/32` to a dummy
interface of the node.  That fake address is reachable only from the pods
running on the node because such pods route all the packets to the node.

Then, run a NAT client pod on another node.  If the NAT client can reach
the fake IP address, we can prove that the Egress NAT feature is working.

Other functions can be tested straightforwardly.

## Implementation

The end-to-end tests are run on [kind, or Kubernetes IN Docker][kind].
kind can create mutli-node clusters without installing CNI plugin.

To run e2e tests, prepare Docker and run the following commands.

```console
$ make start
$ make install-coil
$ make test
```

You may change the default Kubernetes image for kind with `IMAGE` option.

```console
$ make start KUBERNETES_VERSION=1.25.3
```

To stop the cluster, run `make stop`.

[kind]: https://github.com/kubernetes-sigs/kind

> NOTE: Some tests require us to get the IP address from the host's kind
> interface.
> By default first interface with name starting with `br-` will be used.
> If you have more than one interface with such name, you can specify
> which interface should be used by providing an env variable
> `NETWORK_INTERFACE=<name>` to `make test` command, for example:

```
NETWORK_INTERFACE=br-36d13ecf5bde make test
```
