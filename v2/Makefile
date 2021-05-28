# Makefile for Coil

include common.mk

PROTOC_VERSION=3.17.1
CONTROLLER_RUNTIME_VERSION := $(shell awk '/sigs\.k8s\.io\/controller-runtime/ {print substr($$2, 2)}' go.mod)
CONTROLLER_TOOLS_VERSION=0.5.0
PROTOC_GEN_GO_VERSION := $(shell awk '/google.golang.org\/protobuf/ {print substr($$2, 2)}' go.mod)
PROTOC_GEN_GO_GRPC_VERSON=1.1.0
PROTOC_GEN_DOC_VERSION=1.4.1

## DON'T EDIT BELOW THIS LINE
SUDO=sudo
CONTROLLER_GEN := $(PWD)/bin/controller-gen
CRD_OPTIONS = "crd:crdVersions=v1"
ROLES = config/rbac/coil-controller_role.yaml \
	config/rbac/coild_role.yaml \
	config/rbac/coil-router_role.yaml \
	config/rbac/coil-egress_role.yaml
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
PROTOC := PATH=$(PWD)/bin:'$(PATH)' $(PWD)/bin/protoc -I=$(PWD)/include:.
PODNSLIST = pod1 pod2 pod3
NATNSLIST = nat-client nat-router nat-egress nat-target
OTHERNSLIST = test-egress-dual test-egress-v4 test-egress-v6 \
	test-client-dual test-client-v4 test-client-v6 test-client-custom \
	test-fou-dual test-fou-v4 test-fou-v6

# Set the shell used to bash for better error handling.
SHELL = /bin/bash
.SHELLFLAGS = -e -o pipefail -c

# Run tests, and set up envtest if not done already.
ENVTEST_ASSETS_DIR := testbin
ENVTEST_SCRIPT_URL := https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v$(CONTROLLER_RUNTIME_VERSION)/hack/setup-envtest.sh
.PHONY: test
test: simple-test
ifeq (,$(wildcard $(ENVTEST_ASSETS_DIR)/setup-envtest.sh))
	mkdir -p $(ENVTEST_ASSETS_DIR)
	curl -sSLo $(ENVTEST_ASSETS_DIR)/setup-envtest.sh $(ENVTEST_SCRIPT_URL)
endif
	{ \
	source $(ENVTEST_ASSETS_DIR)/setup-envtest.sh && \
	ENVTEST_K8S_VERSION= fetch_envtest_tools $(ENVTEST_ASSETS_DIR) && \
	setup_envtest_env $(PWD)/$(ENVTEST_ASSETS_DIR) && \
	go test -race -v -count 1 ./... ; \
	}

.PHONY: simple-test
simple-test: test-tools
	test -z "$$(gofmt -s -l . | tee /dev/stderr)"
	staticcheck ./...
	nilerr ./...
	go install ./...
	go vet ./...

.PHONY: test-nodenet
test-nodenet:
	go test -c ./pkg/nodenet
	for i in $@ $(PODNSLIST); do $(SUDO) ip netns delete $$i 2>/dev/null || true; done
	for i in $@ $(PODNSLIST); do $(SUDO) ip netns add $$i; done
	for i in $@ $(PODNSLIST); do $(SUDO) ip netns exec $$i ip link set lo up; done
	$(SUDO) ip netns exec $@ ./nodenet.test -test.v
	for i in $@ $(PODNSLIST); do $(SUDO) ip netns delete $$i; done
	rm -f nodenet.test

.PHONY: test-founat
test-founat:
	go test -c ./pkg/founat
	for i in $(NATNSLIST) $(OTHERNSLIST); do $(SUDO) ip netns delete $$i 2>/dev/null || true; done
	for i in $(NATNSLIST) $(OTHERNSLIST); do $(SUDO) ip netns add $$i; done
	for i in $(NATNSLIST) $(OTHERNSLIST); do $(SUDO) ip netns exec $$i ip link set lo up; done
	$(SUDO) ./founat.test -test.v
	#for i in $(NATNSLIST) $(OTHERNSLIST); do $(SUDO) ip netns delete $$i; done
	rm -f founat.test

.PHONY: check-generate
check-generate:
	$(MAKE) manifests
	$(MAKE) generate
	git diff --exit-code --name-only

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: $(CONTROLLER_GEN) $(ROLES)
	$(CONTROLLER_GEN) $(CRD_OPTIONS) webhook paths="./..." output:crd:artifacts:config=config/crd/bases

COIL_CONTROLLER_ROLE_DEPENDS = controllers/addresspool_controller.go \
	controllers/blockrequest_controller.go \
	controllers/egress_controller.go \
	controllers/clusterrolebinding_controller.go \
	pkg/ipam/pool.go \
	runners/garbage_collector.go

config/rbac/coil-controller_role.yaml: $(COIL_CONTROLLER_ROLE_DEPENDS)
	-rm -rf work
	mkdir work
	sed '0,/^package/s/.*/package work/' controllers/addresspool_controller.go > work/addresspool_controller.go
	sed '0,/^package/s/.*/package work/' controllers/blockrequest_controller.go > work/blockrequest_controller.go
	sed '0,/^package/s/.*/package work/' controllers/egress_controller.go > work/egress_controller.go
	sed '0,/^package/s/.*/package work/' controllers/clusterrolebinding_controller.go > work/clusterrolebinding_controller.go
	sed '0,/^package/s/.*/package work/' pkg/ipam/pool.go > work/pool.go
	sed '0,/^package/s/.*/package work/' runners/garbage_collector.go > work/garbage_collector.go
	$(CONTROLLER_GEN) rbac:roleName=coil-controller paths=./work output:stdout > $@
	rm -rf work

COILD_DEPENDS = controllers/blockrequest_watcher.go \
	pkg/ipam/node.go \
	runners/coild_server.go

config/rbac/coild_role.yaml: $(COILD_DEPENDS)
	-rm -rf work
	mkdir work
	sed '0,/^package/s/.*/package work/' controllers/blockrequest_watcher.go > work/blockrequest_watcher.go
	sed '0,/^package/s/.*/package work/' pkg/ipam/node.go > work/node.go
	sed '0,/^package/s/.*/package work/' runners/coild_server.go > work/coild_server.go
	$(CONTROLLER_GEN) rbac:roleName=coild paths=./work output:stdout > $@
	rm -rf work

COIL_ROUTER_DEPENDS = controllers/addressblock_controller.go \
	runners/router.go

config/rbac/coil-router_role.yaml: $(COIL_ROUTER_DEPENDS)
	-rm -rf work
	mkdir work
	sed '0,/^package/s/.*/package work/' controllers/addressblock_controller.go > work/addressblock_controller.go
	sed '0,/^package/s/.*/package work/' runners/router.go > work/router.go
	$(CONTROLLER_GEN) rbac:roleName=coil-router paths=./work output:stdout > $@
	rm -rf work

config/rbac/coil-egress_role.yaml: controllers/pod_watcher.go
	-rm -rf work
	mkdir work
	sed '0,/^package/s/.*/package work/' $< > work/pod_watcher.go
	$(CONTROLLER_GEN) rbac:roleName=coil-egress paths=./work output:stdout > $@
	rm -rf work

# TLS certificates for webhook
.PHONY: certs
certs: config/default/cert.pem config/default/key.pem

config/default/cert.pem config/default/key.pem:
	go run ./cmd/gencert -outdir=$(PWD)/config/default

# Generate code
.PHONY: generate
generate: $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	$(MAKE) pkg/cnirpc/cni.pb.go pkg/cnirpc/cni_grpc.pb.go ../docs/cni-grpc.md

$(CONTROLLER_GEN):
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v$(CONTROLLER_TOOLS_VERSION))

pkg/cnirpc/cni.pb.go: pkg/cnirpc/cni.proto
	$(PROTOC) --go_out=module=github.com/cybozu-go/coil/v2:. $<

pkg/cnirpc/cni_grpc.pb.go: pkg/cnirpc/cni.proto
	$(PROTOC) --go-grpc_out=module=github.com/cybozu-go/coil/v2:. $<

../docs/cni-grpc.md: pkg/cnirpc/cni.proto
	$(PROTOC) --doc_out=../docs --doc_opt=markdown,$@ $<

.PHONY: image
image:
	-rm -rf work
	GOBIN=$(PWD)/work go install -ldflags="-s -w" ./cmd/coil*
	cp ../LICENSE work
	docker build --no-cache -t coil:dev .
	rm -rf work

.PHONY: setup
setup:
	$(SUDO) apt-get update
	$(SUDO) apt-get -y install --no-install-recommends rsync unzip

	curl -sfL -o protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/protoc-$(PROTOC_VERSION)-linux-x86_64.zip
	unzip -o protoc.zip bin/protoc 'include/*'
	rm -f protoc.zip
	GOBIN=$(PWD)/bin go install google.golang.org/protobuf/cmd/protoc-gen-go@v$(PROTOC_GEN_GO_VERSION)
	GOBIN=$(PWD)/bin go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v$(PROTOC_GEN_GO_GRPC_VERSON)
	GOBIN=$(PWD)/bin go install github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@v$(PROTOC_GEN_DOC_VERSION)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

.PHONY: test-tools
test-tools: staticcheck nilerr

.PHONY: staticcheck
staticcheck:
	if ! which staticcheck >/dev/null; then \
		env GOFLAGS= go install honnef.co/go/tools/cmd/staticcheck@latest; \
	fi

.PHONY: nilerr
nilerr:
	if ! which nilerr >/dev/null; then \
		env GOFLAGS= go install github.com/gostaticanalysis/nilerr/cmd/nilerr@latest; \
	fi
