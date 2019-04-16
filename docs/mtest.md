Multi-host Test (mtest)
=======================

[mtest](../mtest/) directory contains test suites to run integration tests.

Type of Test Suites
-------------------

There are following types of test suites.

1. functions

    This suite tests coil controller, coil installer, `coilctl` command and kubernetes workloads deployments.

Each test suite has an entry point of test as `<suite>/suite_test.go`.

Synopsis
--------

[`Makefile`](../mtest/Makefile) setup virtual machine environment and runs mtest.

* `make setup`

    Install mtest required components.

* `make clean`

    Delete generated files in `output/` directory.

* `make placemat`

    Run `placemat` in background by systemd-run to start virtual machines.

* `make stop`

    Stop `placemat`.

* `make test`

    Run mtest on a running `placemat`.

Options
-------

### `SUITE`

You can choose the type of test suite by specifying `SUITE` make variable.
The value can be `functions` (default).

`make test` accepts this variable.

The value of `SUITE` is interpreted as a Go package name.  You can write
a new test suite and specify its package name by `SUITE`.  As a side note,
the forms of `./functions` is more proper.
