Installer for CNI files
=======================

Coil installs a CNI plugin `coil` and a CNI configuration file to host OS
by using `DaemonSet` pod.  The pod has a container to install them.

It enables IP packet forwarding.

The installer looks for the following environment variables:

Name               | Default               | Description
----               | --------------------- | -----------
`CNI_CONF_NAME`    | `10-coil.conflist`    | The filename of the CNI configuration file.
`CNI_ETC_DIR`      | `/host/etc/cni/net.d` | Installation directory for CNI configuration file.
`CNI_BIN_DIR`      | `/host/opt/cni/bin`   | Installation directory for CNI plugin.
`COIL_PATH`        | `/coil`               | Path to `coil`.
`CNI_NETCONF_FILE` |                       | Path to CNI configuration file.
`CNI_NETCONF`      |                       | CNI configuration file contents.
`COIL_NODE_NAME`   |                       | The node name to install `coil`.
`COIL_BOOT_TAINT`  |                       | The key of Taint to delete when the coil is installed.
