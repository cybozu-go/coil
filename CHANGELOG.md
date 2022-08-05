# Change Log

All notable changes to this project will be documented in this file.
This project adheres to [Semantic Versioning](http://semver.org/).

## [Unreleased]

## [2.1.1] - 2022-08-05

### Changed

- Use ginkgo/v2 (#221)
- Add log for delPeer (#223)


## [2.1.0] - 2022-07-12

### Added

- Support Kubernetes 1.23 and update dependencies (#211)
- Support Kubernetes 1.24 (#214)

### Changed

- DualStack Pools not working. (#209)
- Remove sleep from CNI DEL implementation (#215)

### Upgrade Note

- To users of v2.0.6 or earlier, upgrade to v2.0.14 first and then to v2.1.0. Please do not upgrade to v2.1.0 directly.
  - The leader election of controller-runtime may not work as expected during the rolling update of coil if you upgrade to v2.1.0 directly because controller-runtime uses leases as the default resource lock object since v0.12.0. See https://github.com/kubernetes-sigs/controller-runtime/pull/1773
- kube-systme/coil-leader configmap might remain after the upgrade. Please remove it manually.

## [2.0.14] - 2021-12-14

### Changed

- Fix not to create multiple address blocks for a single request (#198)

## [2.0.13] - 2021-10-25

### Added

- Support k8s 1.22 and update dependencies (#191)

### Changed

- Revert "Modify Pod netrowk setup" (#179)
- Update Go to 1.17 (#180)
- Bump code base to CNI 1.0.0 (#181)
- Add node's internal IP to host side veth (#184)

### Removed

- Remove v1 and coil-migrator (#190)
- Drop k8s 1.19 support (#191)

## [2.0.12] - 2021-09-17

### Changed

- Modify Pod netrowk setup (#175)
- Fix coild doesn't release unused blocks from a pool which is not registered (#177)

## [2.0.11] - 2021-08-27

### Added

- Add CNI Version 0.3.1 (#171)

## [2.0.10] - 2021-08-20

### Changed

- Add finalizer for AddressPool to prevent deletion while it is used by some AddressBlock (#168)

## [2.0.9] - 2021-06-30

### Changed

- Wait before destroying pod network for graceful termination (#164)

## [2.0.8] - 2021-06-11

### Added

- Update tools and dependencies, add support for k8s 1.21, drop support for k8s 1.18 (#160)

### Changed

- Use a client that reads directly from a API server when SyncBlocks synchronizes allocated field (#161)
- Update controller-runtime to 0.9.0 (#162)

## [2.0.7] - 2021-04-22

### Changed

- Upgrade kubebuilder to v3 and controller-runtime to v0.8.3 (#154)
  - Also, add node-role.kubernetes.io/control-plane label to tolerations

## [2.0.6] - 2021-04-07

### Added

- Add coil_egress_client_pod_count metrics (#148)
- Add support for k8s 1.20 (#140)

## [2.0.5] - 2021-01-14

### Changed

- egress: mount emptyDir on /run in egress pods (#141)

## [2.0.4] - 2021-01-13

### Added

- Grafana dashboard (#135)

### Changed

- coil-controller: export metrics for all pools (#133)
- build with Go 1.15 on Ubuntu 20.04 (#138)

## [2.0.3] - 2020-11-19

### Added

- Auto MTU configuration (#130).

### Changed

- `coil-migrator`: restart `coild` Pods just before completion (#127).

## [2.0.2] - 2020-10-16

### Changed

- `coild`: register pod routes in a separate table (#125).
- config: fix Pod Security Policy for `coil-egress` (#125).
- `coil-migrator`: wait for StatefulSet's Pod deletion correctly (#124).

## [2.0.1] - 2020-10-12

### Changed

- config: add protocol=TCP to Service for server-side apply (#122).

## [2.0.0] - 2020-10-12

### Added

- `coil-migrator`: a utility to help live migration from v1 to v2 (#119).
- Install option for [CKE](https://github.com/cybozu-go/cke) (#120).

## [2.0.0-rc.1] - 2020-10-05

Coil version 2 is a complete rewrite of Coil version 1.
This is the first release candidate with all the planned features implemented.

[Unreleased]: https://github.com/cybozu-go/coil/compare/v2.1.1...HEAD
[2.1.1]: https://github.com/cybozu-go/coil/compare/v2.1.0...v2.1.1
[2.1.0]: https://github.com/cybozu-go/coil/compare/v2.0.14...v2.1.0
[2.0.14]: https://github.com/cybozu-go/coil/compare/v2.0.13...v2.0.14
[2.0.13]: https://github.com/cybozu-go/coil/compare/v2.0.12...v2.0.13
[2.0.12]: https://github.com/cybozu-go/coil/compare/v2.0.11...v2.0.12
[2.0.11]: https://github.com/cybozu-go/coil/compare/v2.0.10...v2.0.11
[2.0.10]: https://github.com/cybozu-go/coil/compare/v2.0.9...v2.0.10
[2.0.9]: https://github.com/cybozu-go/coil/compare/v2.0.8...v2.0.9
[2.0.8]: https://github.com/cybozu-go/coil/compare/v2.0.7...v2.0.8
[2.0.7]: https://github.com/cybozu-go/coil/compare/v2.0.6...v2.0.7
[2.0.6]: https://github.com/cybozu-go/coil/compare/v2.0.5...v2.0.6
[2.0.5]: https://github.com/cybozu-go/coil/compare/v2.0.4...v2.0.5
[2.0.4]: https://github.com/cybozu-go/coil/compare/v2.0.3...v2.0.4
[2.0.3]: https://github.com/cybozu-go/coil/compare/v2.0.2...v2.0.3
[2.0.2]: https://github.com/cybozu-go/coil/compare/v2.0.1...v2.0.2
[2.0.1]: https://github.com/cybozu-go/coil/compare/v2.0.0...v2.0.1
[2.0.0]: https://github.com/cybozu-go/coil/compare/v2.0.0-rc.1...v2.0.0
[2.0.0-rc.1]: https://github.com/cybozu-go/coil/compare/v1.1.9...v2.0.0-rc.1
