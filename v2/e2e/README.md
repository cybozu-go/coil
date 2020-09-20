End-to-end test suites
======================

This document describes the strategy, analysis, and implementation of
end-to-end (e2e) tests for Coil.

- [Strategy](#strategy)
- [Analysis](#analysis)
  - [Manifests](#manifests)
  - [`coil-controller`](#coil-controller)
  - [`coild`](#coild)
  - [`coil-router`](#coil-router)
- [How to test](#how-to-test)
- [Implementation](#implementation)

## Strategy

Almost all the functions of Coil are well-tested in unit tests and
integration tests.  The exceptions are:

- YAML manifests
- `main` function of each program
- inter-node communication

Therefore, it is enough to cover these functions in e2e tests.

## Analysis

### Manifests

RBAC should carefully be examined.
The other manifests are mostly tested together with other tests.

### `coil-controller`

What the `main` function implements are:

- Leader election
- Admission webhook
- Health probe server
- Metrics server
- Reconciler for BlockRequest
- Garbage collector for orphaned AddressBlock

### `coild`

What the `main` function implements are:

- Health probe server
- Metrics server
- gRPC server for `coil`
- Route exporter
- Persisting IPAM status between restarts

### `coil-router`

What the `main` function implements are:

- Health probe server
- Metrics server
- Routing table setup for inter-node communication

## How to test

Health probe servers can be tested by checking Pod readiness.

Reconciler for BlockRequest in `coil-controller`, gRPC server in `coild`,
and routing table setup in `coil-router` can be tested together by
checking if Pods on different nodes can communicate each other.

Admission webhook can be tested by trying to create an invalid
AddressPool that cannot be checked by OpenAPI validations.
A too narrow subnet is such an example.

Garbage collector in `coil-controller` can be tested by creating
orphaned AddressBlock manually.

Persisting IPAM status in `coild` can be tested by restarting `coild` Pods
and examine the allocation status.

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
$ make start IMAGE=kindest/node:v1.19.1
```

To stop the cluster, run `make stop`.

[kind]: https://github.com/kubernetes-sigs/kind
