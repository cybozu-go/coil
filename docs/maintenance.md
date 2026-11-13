Maintenance
===========

This document describes the routine steps for updating dependencies and supported Kubernetes versions.

All commands below are run from the `v2/` directory unless noted otherwise.

## Update Go module dependencies

1. Update tool versions pinned at the top of `v2/Makefile` if newer releases
   exist: `CONTROLLER_TOOLS_VERSION`, `PROTOC_VERSION`,
   `PROTOC_GEN_GO_GRPC_VERSON`, `PROTOC_GEN_DOC_VERSION`, `YQ_VERSION`,
   `SETUP_ENVTEST_VERSION`, `STATICCHECK_VERSION`, `GOIMPORTS_VERSION`.
   (`CONTROLLER_RUNTIME_VERSION` and `PROTOC_GEN_GO_VERSION` are derived
   automatically from `go.mod`, so they don't need manual edits.) These
   tools are downloaded into `v2/bin/` and are only fetched if missing, so
   remove any already-downloaded binary for a tool whose version you bumped
   (e.g. `rm bin/controller-gen bin/yq`) or `make generate`/`make manifests`
   will keep using the old one.

2. Bump versions in `go.mod` (e.g. `k8s.io/*`, `sigs.k8s.io/controller-runtime`,
   `google.golang.org/grpc`, `google.golang.org/protobuf`), then run:

   ```console
   $ go mod tidy
   ```

3. Regenerate derived code and manifests, and clean up imports — this
   picks up both the `go.mod` bump and the tool version bump from step 1:

   ```console
   $ make generate
   $ make manifests
   $ goimports -w -local github.com/cybozu-go/coil/v2 .
   $ go mod tidy
   ```

   `make check-generate` runs all of the above plus `git diff --exit-code`,
   which is a convenient way to verify nothing was missed.

4. Fix any compile breaks caused by upstream API changes. Major
   `controller-runtime` bumps have previously required hand-edits to the
   webhook types, e.g. `api/v2/addresspool_webhook.go` and
   `api/v2/egress_webhook.go` (see #372).

## Update the Go toolchain version

Keep these three in sync:

- `go` directive in `v2/go.mod`
- `go-version` in `.github/workflows/ci.yaml` and `.github/workflows/release.yaml`
- base image tag in `v2/Dockerfile` (`ghcr.io/cybozu/golang:<major.minor>-<ubuntu codename>`)

## Support a new Kubernetes version

1. Bump `k8s.io/api`, `k8s.io/apimachinery`, `k8s.io/client-go`, and related
   `k8s.io/*`/`sigs.k8s.io/*` modules in `go.mod`, then follow the "Update Go
   module dependencies" steps above.
2. Bump `kubernetes-sigs/kind` in `v2/aqua.yaml` to a release that ships a
   node image for the new version, and bump `kubernetes/kubernetes/kubectl`
   to the new Kubernetes version. Also bump `kubernetes-sigs/kustomize` if a
   newer release is needed. Then regenerate `v2/aqua-checksums.json` (do not
   edit it by hand):

   ```console
   $ aqua update-checksum -prune
   ```

3. Update `K8S_LATEST`, `K8S_PREV1` and `K8S_PREV2` under `env` in
   `.github/workflows/ci.yaml`; every matrix entry refers to them through
   YAML anchors. Coil supports three minor versions at a time, so drop the
   oldest when adding the newest. Take the patch versions from the node
   images listed in the release notes of the kind version chosen in step 2.
4. Update `KUBERNETES_VERSION` in `v2/e2e/Makefile` to the same value as
   `K8S_LATEST`.
5. Update the "Kubernetes Version" line under `## Dependencies` in
   `README.md`.

## Verify

```console
$ make check-generate
$ make test
```
