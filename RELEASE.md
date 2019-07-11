Release procedure
=================

This document describes how to release a new version of coil.

Versioning
----------

Follow [semantic versioning 2.0.0][semver] to choose the new version number.

Prepare change log entries
--------------------------

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

Bump version
------------

1. Determine a new version number.  Let it write `$VERSION`.
1. Checkout `master` branch.
1. Edit `CHANGELOG.md` for the new version ([example][]).
1. Commit the change and add a git tag, then push them.

    ```console
    $ git commit -a -m "Bump version to $VERSION"
    $ git tag v$VERSION
    $ git push origin master v$VERSION
    ```
Now the version is bumped up and the latest container image is uploaded to [quay.io](https://quay.io/cybozu/coil).

Publish GitHub release page
---------------------------

Go to https://github.com/cybozu-go/coil/releases and edit the tag.
Finally, press `Publish release` button.


[semver]: https://semver.org/spec/v2.0.0.html
[example]: https://github.com/cybozu-go/etcdpasswd/commit/77d95384ac6c97e7f48281eaf23cb94f68867f79
