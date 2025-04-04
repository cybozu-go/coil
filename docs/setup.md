How to setup Coil
=================

This document describes step-by-step instructions for installing Coil to Kubernetes.
All the instructions should be done under the `v2/` directory.

The YAML manifests of Coil can be generated using [kustomize](https://kubernetes-sigs.github.io/kustomize/).
You can tweak optional parameters by editing [`kustomization.yaml`](../v2/kustomization.yaml) file.

- [Install `kustomize`](#install-kustomize)
- [TLS certificates](#tls-certificates)
  - [Generate certificates manually](#generate-certificates-manually)
  - [Enable automatic certs rotation](#enable-automatic-certs-rotation)
- [Edit `kustomization.yaml`](#edit-kustomizationyaml)
- [Edit `netconf.json`](#edit-netconfjson)
- [Compile and apply the manifest](#compile-and-apply-the-manifest)
- [Define the default address pool](#define-the-default-address-pool)
  - [IPv6 pool](#ipv6-pool)
  - [IPv4/v6 dual stack pool](#ipv4v6-dual-stack-pool)
- [(Option) Configure BIRD](#option-configure-bird)
- [Note on CRI runtime compatibility](#note-on-cri-runtime-compatibility)
- [Standalone egress](#standalone-egress)
  - [Configuration](#configuration)
  - [Testing standalone egress](#testing-standalone-egress)
    - [Testing with Kindnet using IPv4](#testing-with-kindnet-using-ipv4)
    - [Testing with Kindnet using IPv6](#testing-with-kindnet-using-ipv6)

## Install `kustomize`

Follow the instructions: https://kubectl.docs.kubernetes.io/installation/kustomize/

`kustomize` 4.1.3 is verified to work for Coil.

## TLS certificates

Coil runs admission webhook servers, and each one needs a self-signed certificate. You can either generate certificates manually or have Coil create them when it starts up.

### Generate certificates manually

Run `make certs` under `v2/` directory to generate the certificates.

```console
$ make certs
```

This should generate the following PEM files:

```console
$ ls config/default/*.pem
config/default/cert.pem         config/default/egress-key.pem  config/default/ipam-key.pem
config/default/egress-cert.pem  config/default/ipam-cert.pem   config/default/key.pem
```

### Enable automatic certs rotation

Run `make enable-certs-rotation` under `v2/` directory to enable automatic certificate generation in `coil`.

```console
$ make enable-certs-rotation
```

This will configure the kustomization files that will can be later used by `make install-coil` target.

## Edit `kustomization.yaml`

`kustomization.yaml` under `v2/` directory contains some commented option settings.
Uncomment if you want to enable them.

If all the nodes are connected in a flat L2 network, enabling `coil-router` is recommended.

```console
$ vi kustomization.yaml
```

## Edit `netconf.json`

`netconf.json` under `v2/` directory is just an example [CNI network configuration][netconf]
(actually, the example is a network configuration list).

You may edit the file to, say, add Cilium for network policies or to tune MTU.
Note that `coil` must be the first in the plugin list if IPAM is enabled.

```console
vi netconf.json
```

These documents help you to edit the configuration.
- [Network Plugins](https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/network-plugins/#cni)
- [tuning plugin](https://github.com/containernetworking/plugins/tree/master/plugins/meta/tuning)

The following example adds `tuning` and `bandwidth` plugins.

```json
{
  "cniVersion": "0.4.0",
  "name": "k8s-pod-network",
  "plugins": [
    {
      "type": "coil",
      "socket": "/run/coild.sock",
    },
    {
      "type": "tuning",
      "mtu": 1400
    },
    {
      "type": "bandwidth",
      "capabilities": {
        "bandwidth": true
      }
    },
    {
      "type": "portmap",
      "capabilities": {
        "portMappings": true
      }
    }
  ]
}
```

## Compile and apply the manifest

Now you can compile the manifest with `kustomize` and apply it to your cluster.

```console
$ kustomize build . > coil.yaml
$ kubectl apply -f coil.yaml
```

## Define the default address pool

There should be the default address pool for Pods.
Create a YAML manifest like this and apply it.

```yaml
apiVersion: coil.cybozu.com/v2
kind: AddressPool
metadata:
  name: default
spec:
  blockSizeBits: 5
  subnets:
  - ipv4: 10.100.0.0/16
```

- `blockSizeBits`: 5 means that blocks of 32 (= 2^5) addresses will be carved out.
- `subnets`: a list of IP subnets in this pool.

### IPv6 pool

To define an IPv6 address pool, change the `spec` as follows:

```yaml
spec:
  blockSizeBits: 8
  subnets:
  - ipv6: fd02::/96
```

### IPv4/v6 dual stack pool

To define an IPv4/v6 dual stack pool, change the `spec` as follows:

```yaml
spec:
  blockSizeBits: 5
  subnets:
  - ipv4: 10.100.0.0/16
    ipv6: fd02::/112
```

Note that IPv4 and IPv6 subnet must be the same size.
In the above example, their sizes are 16 bits.

## (Option) Configure BIRD

If your nodes are not connected each other within a flat L2 network, or
if you did not enable `coil-router` in `kustomization.yaml`, you need to
configure some routing software to advertise routes.

Coil exports routing information in an unused kernel routing table.
By default, the routing table ID is 119.

Routing software should import routes from the table.  Following
is a configuration snippet for [BIRD][].

```
# export4 is a table to be used for routing protocol such as BGP
ipv4 table export4;

# Import Coil routes into export4
protocol kernel 'coil' {
    kernel table 119;  # the routing table coild exports routes.
    learn;
    scan time 1;
    ipv4 {
        table export4;
        import all;
        export none;
    };
}

# Import routes from external routers into master4, the main table for IPv4 routes
protocol pipe {
    table master4;
    peer table export4;
    import filter {
        if proto = "coil" then reject;
        accept;
    };
    export none;
}

# Reflect routes in master4 into the Linux kernel
protocol kernel {
    ipv4 {
        export all;
    };
}

# This is merely an example of using BGP to exchange routes between external routers
protocol bgp {
    local as __ASN__;
    neighbor __PEER_ADDRESS__ as __PEER_ASN__;

    ipv4 {
        table export4;
        import all;
        export all;
        next hop self;
    };
}
```

## Note on CRI runtime compatibility

`coild` needs to see the network namespace (netns) files on the host.

Such files are usually created under `/proc`.
`coild` shares the PID namespace to see netns files under `/proc`, so
if your CRI runtime passes file path under `/proc` to CNI plugins,
there is no problem.

Some CRI runtimes are known to bind mount netns files under `/var/run/netns`.
The default manifest of `coild` mounts that host directory, so if your CRI
runtime passes file path under `/var/run/netns`, there is also no problem.

Otherwise, `coild` might not work with your CRI runtime.
In such cases, you need to edit `config/pod/coild.yaml` to mount the right
host directory.

[netconf]: https://github.com/containernetworking/cni/blob/spec-v0.4.0/SPEC.md#network-configuration
[BIRD]: https://bird.network.cz/

## Standalone egress

Coil can be run as standalone egress NAT controller, using CNI chaining with another CNI providing base connectivity. This chapter will guide you on how to achieve this.

### Configuration

To deploy Coil with only egress feature enabled the following changes are required in the configuration files:

1. Comment all IPAM related pieces in the following `kustomization.yaml` files:
    - `v2/config/crd/kustomization.yaml`
    - `v2/config/default/kustomization.yaml`
    - `v2/config/pod/kustomization.yaml`
    - `v2/config/rbac/kustomization.yaml`

1. Comment unnecessary resources in `config/crd/patches/remove_status.yaml`.
1. Add following arguments to the `coild` contianer executable in `config/pod/coild.yaml`
    ```yaml
    containers:
      - name: coild
        image: coil:dev
        command: ["coild"]
        args:
          - --zap-stacktrace-level=panic
          - --enable-ipam=false
          - --enable-egress=true
          - --pod-table-id=0 # 255 if IPv6 is being used
          - --protocol-id=2
    ```
1. Set CNI config filename using environment variable for init contianer `coil-installer` in `config/pod/coild.yaml`:
    ```yaml
    env:
    - name: CNI_CONF_NAME
      value: "01-coil.conflist"
    ```
1. Add configuration of your chosen CNI to `v2/netconf.json` before `coil` related configuration.
1. Deploy `coil` to existing cluster as described in [Compile and apply the manifest](#compile-and-apply-the-manifest).

### Testing standalone egress

#### Testing with Kindnet using IPv4
1. Generate certificates using `v2/Makefile`.
    ```bash
    cd v2 && make certs
    ```
1. Go to `v2/e2e`
    ```bash
    cd e2e
    ```
1. Create IPv4 based Kind cluster with Kindnet CNI deployed:
    ```bash
    WITH_KINDNET=true TEST_IPV6=false make start
    ```
1. Install Coil on the cluster:
    ```bash
    make install-coil-egress-v4
    ```
1. Run egress-only IPv4 tests:
    ```bash
    TEST_IPAM=false TEST_EGRESS=true TEST_IPV6=false make test
    ```

#### Testing with Kindnet using IPv6
1. Generate certificates using `v2/Makefile`.
    ```bash
    cd v2 && make certs
    ```
1. Go to `v2/e2e`
    ```bash
    cd e2e
    ```
1. Create IPv6 based Kind cluster with Kindnet CNI deployed:
    ```bash
    WITH_KINDNET=true TEST_IPV6=true make start
    ```
1. Install Coil on the cluster:
    ```bash
    make install-coil-egress-v6
    ```
1. Run egress-only IPv6 tests:
    ```bash
    TEST_IPAM=false TEST_EGRESS=true TEST_IPV6=true make test
    ```
