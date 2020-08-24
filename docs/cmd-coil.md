coil
====

`coil` command is a [CNI plugin](https://github.com/containernetworking/cni/blob/spec-v0.4.0/SPEC.md#cni-plugin).

It delegates all requests except for `VERSION` to `coild` through gRPC over UNIX domain socket.
The default socket path is `/run/coild.sock`, but it can be changed with `socket` parameter in the network configuration as follows:

```json
{
  "cniVersion": "0.4.0",
  "name": "k8s",
  "type": "coil",
  "socket": "/tmp/coild.sock"
}
```
