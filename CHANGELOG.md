# Change Log

All notable changes to this project will be documented in this file.
This project adheres to [Semantic Versioning](http://semver.org/).

## [Unreleased]

## [2.8.0] - 2025-01-07

### Added
- Support k8s 1.30 and dependencies (#301)
- Add conntrack command into the coil image (#305)
- Add egress related metrics (#306)
  - See issue [#307](https://github.com/cybozu-go/coil/issues/307) for details

## [2.7.2] - 2024-08-09

- Add info logs to egress (#298)

### Changed

## [2.7.1] - 2024-07-22

### Changed

- docs: Remove git-neco (#295)
- Update dependencies (#294)
- Bump google.golang.org/grpc from 1.64.0 to 1.64.1 in /v2 (#293)

## [2.7.0] - 2024-06-06

### Changed

- Support Kubernetes 1.29 and CNI 1.1.0 (#290)

## [2.6.1] - 2024-05-28

### Changed

- Fix downtime on NAT client startup (#288)
- Make setup egress before the manager starts (#286)
- Support Kubernetes 1.28 (#285)
- Fix to be able to remove AddressPool finalizer by controller (#283)
- Bump golang.org/x/net from 0.22.0 to 0.23.0 in /v2 (#282)
- Add version info to the starting logs (#281)

## [2.6.0] - 2024-04-11

### Added

- Support PDB for egress (#275)

### Changed

- Fix to check that egress_watcher pick a valid client (#280)
- Update dependencies (#278)
- Bump google.golang.org/protobuf from 1.31.0 to 1.33.0 in /v2 (#277)

## [2.5.2] - 2024-02-02

### Changed

- Fix to avoid adding FoU devices for pods that don't use its egress (#265)
- Remove a log for v1 migration error (#266)
- Refactor egress watcher related logs (#267)

## [2.5.1] - 2023-11-27

### Changed

- Update gRPC modules (#259)
- Update dependencies (#261)
- Add logs (#262)

## [2.5.0] - 2023-10-26

### Added

- Support k8s 1.27 and update dependencies (#254)
- Support for updating NAT client configuration (#253)
- Support automatic source port selection in UDP encapsulation (#252)

### Changed

- Migrate to go-grpc-middleware v2 (#255)

## [2.4.0] - 2023-07-10

### Added

- Support k8s 1.26 and update dependencies (#246)

## [2.3.0] - 2023-02-17

### Added

- Added an arm64 image for coil (#226)

## Changed

- Remove replace directive (#241)

## [2.2.0] - 2023-01-24

### Added

- Support Kubernetes 1.25 (#237)

## [2.1.4] - 2023-01-12

### Changed

- Add ipam document (#233)
- Fix IPAM logic not to reuse the same addresses immediately (#234)

## [2.1.3] - 2022-10-25

### Changed

- Update dependencies (#230)

## [2.1.2] - 2022-09-15

### Changed

- Fix the bug that Coil accidentally deletes a live peer (#227)

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

[Unreleased]: https://github.com/cybozu-go/coil/compare/v2.8.0...HEAD
[2.8.0]: https://github.com/cybozu-go/coil/compare/v2.7.2...v2.8.0
[2.7.2]: https://github.com/cybozu-go/coil/compare/v2.7.1...v2.7.2
[2.7.1]: https://github.com/cybozu-go/coil/compare/v2.7.0...v2.7.1
[2.7.0]: https://github.com/cybozu-go/coil/compare/v2.6.1...v2.7.0
[2.6.1]: https://github.com/cybozu-go/coil/compare/v2.6.0...v2.6.1
[2.6.0]: https://github.com/cybozu-go/coil/compare/v2.5.2...v2.6.0
[2.5.2]: https://github.com/cybozu-go/coil/compare/v2.5.1...v2.5.2
[2.5.1]: https://github.com/cybozu-go/coil/compare/v2.5.0...v2.5.1
[2.5.0]: https://github.com/cybozu-go/coil/compare/v2.4.0...v2.5.0
[2.4.0]: https://github.com/cybozu-go/coil/compare/v2.3.0...v2.4.0
[2.3.0]: https://github.com/cybozu-go/coil/compare/v2.2.0...v2.3.0
[2.2.0]: https://github.com/cybozu-go/coil/compare/v2.1.4...v2.2.0
[2.1.4]: https://github.com/cybozu-go/coil/compare/v2.1.3...v2.1.4
[2.1.3]: https://github.com/cybozu-go/coil/compare/v2.1.2...v2.1.3
[2.1.2]: https://github.com/cybozu-go/coil/compare/v2.1.1...v2.1.2
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
