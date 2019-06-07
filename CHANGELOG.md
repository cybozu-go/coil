# Change Log

All notable changes to this project will be documented in this file.
This project adheres to [Semantic Versioning](http://semver.org/).

## [Unreleased]

## [1.1.2-beta] - 2019-06-07

### Changed

- Pre-release.

## [1.1.1] - 2019-06-07

### Changed

- The naming of veth pairs synchronizes with that of Calico's NetworkPolicy implementation (#45).

## [1.1.0] - 2019-05-10

### Changed

- circleci: do not push branch tag for pre-releases (#52).
- coild: fix error handling (#53).
- Manage resources by container-id and fix potential race condition and http statuses of coild (#54).
- Run `nilerr` and `restrictpkg` for testing and fix running multiple etcd (#56).

## [1.0.2] - 2019-04-19

### Changed

- installer: remove existing CNI files if any (#49).

## [1.0.1] - 2019-02-26

### Changed

- Improve an error message when address pool is not found. (#40)

## [1.0.0] - 2019-01-31

### Changed

- Support for Kubernetes 1.13 (#36)
- `COIL_NODE_IP` is no longer needed for coild

## [0.4] - 2019-01-11

### Changed

- Support for kubernetes 1.12 (#30)
- Update mtest (#27, #28, #31, #33, #34)
- [coil installer] Wait for kubelet to rescan /etc/cni/net.d

## [0.3] - 2018-10-31

### Added

- `hypercoil`: all-in-one binary integrating all coil commands (#25)

## [0.2] - 2018-10-29

### Added

- [coil installer] Remove bootstrap taint from the node when coil is installed (#21)
- [coilctl] accept config from environment variables (#24)

### Changed

- Rename cybozu-go/cmd -> cybozu-go/well

## [0.1] - 2018-10-19

This is the first release

### Added

- Implement CNI plugin, coild, coilctl and coil-controller

[Unreleased]: https://github.com/cybozu-go/coil/compare/v1.1.1...HEAD
[1.1.1]: https://github.com/cybozu-go/coil/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/cybozu-go/coil/compare/v1.0.2...v1.1.0
[1.0.2]: https://github.com/cybozu-go/coil/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/cybozu-go/coil/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/cybozu-go/coil/compare/v0.4...v1.0.0
[0.4]: https://github.com/cybozu-go/coil/compare/v0.3...v0.4
[0.3]: https://github.com/cybozu-go/coil/compare/v0.2...v0.3
[0.2]: https://github.com/cybozu-go/coil/compare/v0.1...v0.2
[0.1]: https://github.com/cybozu-go/coil/compare/91f0cb8b46e800f41a6b811fce811977ac52b07d...v0.1
