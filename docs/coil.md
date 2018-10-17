`coil`
======

`coil` is an executable that implements [CNI][] specification.

It communicates with [`coild`](coild.md) through HTTP REST API.

A sample netconf looks like:
```json
{
  "cniVersion": "0.3.1",
  "name": "k8s",
  "plugins": [
    {
      "type": "coil",
      "coild": "http://127.0.0.1:9383"
    },
    {
      "type": "tuning",
      "mtu": 1400
    },
    {
      "type": "portmap",
      "snat": true,
      "capabilities": {"portMappings": true}
    }
  ]
}
```

Each parameter has a default value:

Name    | Default                 | Description
------- | ----------------------- | -----------
`coild` | "http://127.0.0.1:9383" | `coild` endpoint URL.

[CNI]: https://github.com/containernetworking/cni/blob/spec-v0.3.1/SPEC.md
