`coil`
======

`coil` is an executable that implements [CNI][] specification.

It communicates with [`coild`](coild.md) through HTTP REST API.
The endpoint of `coild` can be configured with CNI netconf JSON as follows:

```json
{
    "cniVersion": "0.3.1",
    "name": "k8s",
    "type": "coil",
    "host-interface": "%%INTERFACE%%",
    "coild": "http://127.0.0.1:9383"
}
```

Options:

Name    | Default                 | Description
------- | ----------------------- | -----------
`coild` | "http://127.0.0.1:9383" | `coild` endpoint URL.

[CNI]: https://github.com/containernetworking/cni/blob/spec-v0.3.1/SPEC.md
