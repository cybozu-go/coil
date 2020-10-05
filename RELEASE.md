Release procedure
=================

This document describes how to release a new version of Coil.

## Versioning

Follow [semantic versioning 2.0.0][semver] to choose the new version number.

## Prepare change log entries

Add notable changes since the last release to [CHANGELOG.md](CHANGELOG.md).
It should look like:

```markdown
(snip)
## [Unreleased]

### Added
- Implement ... (#35)

### Changed
- Fix a bug in ... (#33)

### Removed
- Deprecated `-option` is removed ... (#39)

(snip)
```

## Adding and removing supported Kubernetes versions

- Edit [`.github/workflows/ci.yaml`](.github/workflows/ci.yaml) and edit `kindtest-node` field values.
- Edit Kubernetes versions in [`README.md`](README.md).
- If the minimal supported version is to be changed, edit `v2/common.mk` too.
- Make sure that the changes pass CI.

You should also update `sigs.k8s.io/controller-runtime` Go package periodically.

## Bump version

1. Determine a new version number.  Let it write `$VERSION` as `VERSION=x.y.z`.
2. Checkout `master` branch.
3. Make a branch to release, for example by `git neco dev "bump-$VERSION"`
4. Edit `CHANGELOG.md` for the new version ([example][]).
5. Edit `v2/version.go` for the new version.
6. Edit `v2/kustomization.yaml` and update `newTag` value for the new version.
7. Commit the changes and push it.

    ```console
    $ git commit -a -m "Bump version to $VERSION"
    $ git neco review
    ```
8. Merge this branch.
9. Checkout `master` branch.
10. Add a git tag, then push it.

    ```console
    $ git tag "v$VERSION"
    $ git push origin "v$VERSION"
    ```

Now the version is bumped up and the latest container image is uploaded to GitHub container registry.

## (Option) Edit GitHub release page

If you would like to give further descriptions of the new release,
edit [the GitHub release page](https://github.com/cybozu-go/coil/releases/latest).

[semver]: https://semver.org/spec/v2.0.0.html
[example]: https://github.com/cybozu-go/etcdpasswd/commit/77d95384ac6c97e7f48281eaf23cb94f68867f79
