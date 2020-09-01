coil-installer
==============

`coil-installer` is a program intended to be run as an init container of
`coild` DaemonSet.

It installs `coil` CNI binary and network configuration file into the host OS.

## Environment variables

The installer references the following environment variables:

| Name               | Default               | Description                                        |
| ------------------ | --------------------- | -------------------------------------------------- |
| `CNI_CONF_NAME`    | `10-coil.conflist`    | The filename of the CNI configuration file.        |
| `CNI_ETC_DIR`      | `/host/etc/cni/net.d` | Installation directory for CNI configuration file. |
| `CNI_BIN_DIR`      | `/host/opt/cni/bin`   | Installation directory for CNI plugin.             |
| `COIL_PATH`        | `/coil`               | Path to `coil`.                                    |
| `CNI_NETCONF_FILE` |                       | Path to CNI configuration file.                    |
| `CNI_NETCONF`      |                       | CNI configuration file contents.                   |
