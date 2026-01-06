# Change Log

All notable changes to this project will be documented in this file.
This project adheres to [Semantic Versioning](http://semver.org/).

## [Unreleased]

## [2.13.0] - 2026-01-06

### Changed

- Refactor egress NAT server functionality (#352)
- Fix duplicate nftables rules by using UserData as rule identifier and simplify iptables rule addition with AppendUnique (#355)
- Fix originatingonly configuration (#356)
- Support Kubernetes 1.33 (#359)

## [2.12.0] - 2025-11-28

### Added

- Egress only for connections originating in the client pod (#337)
- Add cross-platform support for yq binary download (#341)
- Add nftables support for egress NAT functionality (#345)
- Feat nftables (#346)

### Changed

- Fix: Add multi-platform support for e2e test binary downloads in Makefile (#338)
- Remove unnecessary kube-proxy double NAT workaround (#340)
- Fix: flaky garbage collection tests by running GC immediately on start (#343)
- Fix CRD generation in Makefile (#344)
- Removed leader election requirement for cert-controller's rotator (#349)
- Update dependencies and introduce goimports (#351)

## [2.11.1] - 2025-07-22

### Changed

- update dependencies (#335)

## [2.11.0] - 2025-06-18

Coil Egress now offers dual-stack support, enabling it to handle both IPv4 and IPv6 traffic within a single Egress. We have also implemented a new iptables rule in Egress pods (which are managed by the Egress resource and function as gateways to external networks) to drop invalid packets. Additionally, Egress-related metrics have been updated:

- New metrics have been added to count invalid packets dropped by this new iptables rule.
- The coil_egress_nftables_masqueraded_packets and coil_egress_nftables_masqueraded_bytes metrics have been updated to include support for IPv6.

This release doesn't have any breaking changes.

### Added

- DualStack support for egress (#238)
- egress: Add new iptabels rule for dropping invalid packets (#329)
- Introduce egress metrics to count invalid packets and bytes (#331)
- Support k8s 1.32 (#332)
- Support IPv6 for egress metrics (#333)

## [2.10.1] - 2025-04-18

### Changed

- Fixed enable-certs-rotation makefile target (#324)
- Test with Ordered option (#326)

## [2.10.0] - 2025-04-09

### Added

- Added cert-controller for easy webhook cert rotation (#319)

## [2.9.1] - 2025-03-28

### Changed

- Update dependencies (#321)

## [2.9.0] - 2025-03-07

### Important Changes

We are excited to introduce the stand-alone egress NAT feature (#299) from this version!
Thank you @p-strusiewiczsurmacki-mobica!

Now you can use stand-alone egress and/or ipam mode by `coild` flags.
By default, both are enabled.

You can configure this settings via coild's flag.
The document is here: [docs/cmd-coild.md](./docs/cmd-coild.md#command-line-flags).

There are no breaking changes in this version.
But there are some points which we should take care of when upgrading.

From this version, `coil-controller` is separated into `coil-ipam-controller` and `coil-egress-controller`.

The following steps show how to upgrade coil v2.9.0.

#### Step1: Generate new manifests

You need to generate new version's manifests.
For example, you can get manifests.

```console
$ cd v2
$ make certs
$ kustomize build --load-restrictor=LoadRestrictionsNone .
```

Due to the controller separation, validating and mutating webhooks have also been changed.
Therefore, when generating new versions of the manifests, you must renew webhooks certificates.

#### Step2: Apply new manifests

Once new manifests are applied, `coil-ipam-controller` and `coil-egress-controller` are deployed.
However, `coil-ipam-controller` stays `Pending` because it uses the same port as `coil-controller`.

The following is an example of the `coil-ipam-controller` stays a pending state.
```console
$ kubectl get pod -n kube-system
NAME                                         READY   STATUS              RESTARTS      AGE
coil-controller-7486bd8778-6jnrv             1/1     Running             0             7m29s
coil-controller-7486bd8778-k6vsq             1/1     Running             1 (76s ago)   7m29s
coil-egress-controller-5f6dccb47b-4g9wv      1/1     Running             0             87s
coil-egress-controller-5f6dccb47b-h4sph      1/1     Running             0             87s
coil-ipam-controller-bcfb666d4-42w52         0/1     Pending             0             87s
coil-ipam-controller-bcfb666d4-n5r6s         0/1     Pending             0             87s
```

#### Step3: Delete coil-controller and old webhooks

The following `coil-controller` related resources are no longer needed, and you must delete these to make coil work well.

- deployment/kube-system/coil-controller
- lease/kube-system/coil-leader
- validatingwebhookconfiguration/coilv2-validating-webhook-configuration
- mutatingwebhookconfiguration/coilv2-mutating-webhook-configuration
- clusterrole/coil-controller
- clusterrolebinding/coil-controller

### Added
- Standalone Egress NAT (#299)
- Add periodic nodeIPAM GC (#309)
- Update for k8s 1.31 (#310)


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

[Unreleased]: https://github.com/cybozu-go/coil/compare/v2.13.0...HEAD
[2.13.0]: https://github.com/cybozu-go/coil/compare/v2.12.0...v2.13.0
[2.12.0]: https://github.com/cybozu-go/coil/compare/v2.11.1...v2.12.0
[2.11.1]: https://github.com/cybozu-go/coil/compare/v2.11.0...v2.11.1
[2.11.0]: https://github.com/cybozu-go/coil/compare/v2.10.1...v2.11.0
[2.10.1]: https://github.com/cybozu-go/coil/compare/v2.10.0...v2.10.1
[2.10.0]: https://github.com/cybozu-go/coil/compare/v2.9.1...v2.10.0
[2.9.1]: https://github.com/cybozu-go/coil/compare/v2.9.0...v2.9.1
[2.9.0]: https://github.com/cybozu-go/coil/compare/v2.8.0...v2.9.0
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
