# Change Log

All notable changes to this project will be documented in this file.
This project adheres to [Semantic Versioning](http://semver.org/).

## [Unreleased]

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

[Unreleased]: https://github.com/cybozu-go/coil/compare/v2.0.5...HEAD
[2.0.5]: https://github.com/cybozu-go/coil/compare/v2.0.4...v2.0.5
[2.0.4]: https://github.com/cybozu-go/coil/compare/v2.0.3...v2.0.4
[2.0.3]: https://github.com/cybozu-go/coil/compare/v2.0.2...v2.0.3
[2.0.2]: https://github.com/cybozu-go/coil/compare/v2.0.1...v2.0.2
[2.0.1]: https://github.com/cybozu-go/coil/compare/v2.0.0...v2.0.1
[2.0.0]: https://github.com/cybozu-go/coil/compare/v2.0.0-rc.1...v2.0.0
[2.0.0-rc.1]: https://github.com/cybozu-go/coil/compare/v1.1.9...v2.0.0-rc.1
