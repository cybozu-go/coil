# Makefile for Coil

include common.mk

KUBEBUILDER_VERSION = 2.3.1
PROTOC_VERSION=3.12.4

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
GO111MODULE=on
KUBEBUILDER_ASSETS := $(PWD)/bin
PROTOC := PATH=$(PWD)/bin:$(PATH) $(PWD)/bin/protoc -I=$(PWD)/include:.
PODNSLIST = pod1 pod2 pod3
NATNSLIST = nat-client nat-router nat-egress nat-target
OTHERNSLIST = test-egress-dual test-egress-v4 test-egress-v6 \
	test-client-dual test-client-v4 test-client-v6 test-client-custom \
	test-fou-dual test-fou-v4 test-fou-v6
export GO111MODULE KUBEBUILDER_ASSETS

.PHONY: test
test: test-tools
	go test -race -v -count 1 ./...
	go install ./...
	go vet ./...
	test -z $$(gofmt -s -l . | tee /dev/stderr)
	staticcheck ./...
	nilerr ./...

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

	# workaround for CRD issue with k8s 1.18 & controller-gen 0.3
	# ref: https://github.com/kubernetes/kubernetes/issues/91395
	sed -i -r 's/^( +)description: Protocol for port\. Must be UDP, TCP, or SCTP\. Defaults to "TCP"\./\0\n\1default: TCP/' \
	  config/crd/bases/coil.cybozu.com_egresses.yaml

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
	$(MAKE) setup

pkg/cnirpc/cni.pb.go: pkg/cnirpc/cni.proto
	$(PROTOC) --go_out=module=github.com/cybozu-go/coil/v2:. $<

pkg/cnirpc/cni_grpc.pb.go: pkg/cnirpc/cni.proto
	$(PROTOC) --go-grpc_out=module=github.com/cybozu-go/coil/v2:. $<

../docs/cni-grpc.md: pkg/cnirpc/cni.proto
	$(PROTOC) --doc_out=../docs --doc_opt=markdown,$@ $<

.PHONY: image
image:
	-rm -rf work
	mkdir -p work/var
	ln -s /run work/var/run
	CGO_ENABLED=0 GOBIN=$(PWD)/work go install -ldflags="-s -w" ./cmd/coil*
	cp ../LICENSE work
	docker build --no-cache -t coil:dev .
	rm -rf work

.PHONY: setup
setup:
	$(SUDO) apt-get update
	$(SUDO) apt-get -y install --no-install-recommends rsync unzip

	mkdir -p bin
	curl -sfL https://go.kubebuilder.io/dl/$(KUBEBUILDER_VERSION)/$(GOOS)/$(GOARCH) | tar -xz -C /tmp/
	mv /tmp/kubebuilder_$(KUBEBUILDER_VERSION)_$(GOOS)_$(GOARCH)/bin/* bin/
	rm -rf /tmp/kubebuilder_*
	GOBIN=$(PWD)/bin go install sigs.k8s.io/controller-tools/cmd/controller-gen
	# Replace bundled kube-apiserver with that of the minimal supported version
	rm -rf tmp && mkdir -p tmp
	curl -sfL https://github.com/kubernetes/kubernetes/archive/v$(K8S_VERSION).tar.gz | tar zxf - -C tmp
	mv tmp/kubernetes-$(K8S_VERSION) tmp/kubernetes
	cd tmp/kubernetes; make all WHAT="cmd/kube-apiserver"
	mv tmp/kubernetes/_output/bin/kube-apiserver bin/
	rm -rf tmp

	curl -sfL -o protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/protoc-$(PROTOC_VERSION)-linux-x86_64.zip
	unzip -o protoc.zip bin/protoc 'include/*'
	rm -f protoc.zip
	GOBIN=$(PWD)/bin go install google.golang.org/protobuf/cmd/protoc-gen-go
	GOBIN=$(PWD)/bin go install google.golang.org/grpc/cmd/protoc-gen-go-grpc
	GOBIN=$(PWD)/bin go install github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc

.PHONY: test-tools
test-tools: staticcheck nilerr

.PHONY: staticcheck
staticcheck:
	if ! which staticcheck >/dev/null; then \
		cd /tmp; env GOFLAGS= GO111MODULE=on go get honnef.co/go/tools/cmd/staticcheck; \
	fi

.PHONY: nilerr
nilerr:
	if ! which nilerr >/dev/null; then \
		cd /tmp; env GOFLAGS= GO111MODULE=on go get github.com/gostaticanalysis/nilerr/cmd/nilerr; \
	fi
