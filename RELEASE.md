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
- Make sure that the changes pass CI.

You should also update `sigs.k8s.io/controller-runtime` Go package periodically.

## Bump version

1. Determine a new version number. Then set `VERSION` variable.

    ```console
    # Set VERSION and confirm it. It should not have "v" prefix.
    $ VERSION=x.y.x
    $ echo $VERSION
    ```

2. Make a branch to release

    ```console
    $ git neco dev "$VERSION"
    ```

3. Edit `CHANGELOG.md` for the new version ([example][]).
4. Edit `v2/version.go` for the new version.
5. Edit `v2/kustomization.yaml` and update `newTag` value for the new version.
6. Commit the changes and push it.

    ```console
    $ git commit -a -m "Bump version to $VERSION"
    $ git neco review
    ```

7. Merge this branch.
8.  Add a git tag to the main HEAD, then push it.

    ```console
    # Set `VERSION` again.
    $ VERSION=x.y.x
    $ echo $VERSION

    $ git checkout main
    $ git pull
    $ git tag -a -m "Release v$VERSION" "v$VERSION"

    # Make sure the release tag exists.
    $ git tag -ln | grep $VERSION

    $ git push origin "v$VERSION"
    ```

GitHub actions will build and push artifacts such as container images and
create a new GitHub release.

[semver]: https://semver.org/spec/v2.0.0.html
[example]: https://github.com/cybozu-go/etcdpasswd/commit/77d95384ac6c97e7f48281eaf23cb94f68867f79
