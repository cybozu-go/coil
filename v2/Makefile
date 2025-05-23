# Makefile for Coil

IMAGE_TAG := latest
CONTROLLER_RUNTIME_VERSION := $(shell awk '/sigs\.k8s\.io\/controller-runtime/ {print substr($$2, 2)}' go.mod)
CONTROLLER_TOOLS_VERSION=0.18.0
PROTOC_VERSION=31.1
PROTOC_GEN_GO_VERSION := $(shell awk '/google.golang.org\/protobuf/ {print substr($$2, 2)}' go.mod)
PROTOC_GEN_GO_GRPC_VERSON=1.5.1
PROTOC_GEN_DOC_VERSION=1.5.1
ARCH := $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
YQ_VERSION=4.45.4

## DON'T EDIT BELOW THIS LINE
SUDO=sudo
CONTROLLER_GEN := $(shell pwd)/bin/controller-gen
SETUP_ENVTEST := $(shell pwd)/bin/setup-envtest
YQ := $(shell pwd)/bin/yq
CRD_OPTIONS = "crd:crdVersions=v1"
ROLES = config/rbac/coil-ipam-controller_role.yaml \
	config/rbac/coil-ipam-controller-certs_role.yaml \
	config/rbac/coil-egress-controller_role.yaml \
	config/rbac/coil-egress-controller-certs_role.yaml \
	config/rbac/coild_role.yaml \
	config/rbac/coil-router_role.yaml \
	config/rbac/coil-egress_role.yaml
PROTOC_OUTPUTS = pkg/cnirpc/cni.pb.go pkg/cnirpc/cni_grpc.pb.go ../docs/cni-grpc.md
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
PROTOC := PATH=$(PWD)/bin:'$(PATH)' $(PWD)/bin/protoc -I=$(PWD)/include:.
PODNSLIST = pod1 pod2 pod3 pod4 pod5 pod6
NATNSLIST = nat-client nat-router nat-egress nat-target
OTHERNSLIST = test-egress-dual test-egress-v4 test-egress-v6 \
	test-client-dual test-client-v4 test-client-v6 test-client-custom \
	test-client-dual-oo test-client-v4-oo test-client-v6-oo test-client-custom-oo \
	test-fou-dual test-fou-v4 test-fou-v6
WGET_OPTIONS := --retry-on-http-error=503 --retry-connrefused --no-verbose
WGET := wget $(WGET_OPTIONS)

# Set the shell used to bash for better error handling.
SHELL = /bin/bash
.SHELLFLAGS = -e -o pipefail -c

# Run tests, and set up envtest if not done already.
.PHONY: test
test: simple-test setup-envtest
	source <($(SETUP_ENVTEST) use -p env); \
		TEST_IPAM=$(TEST_IPAM) TEST_EGRESS=$(TEST_EGRESS) go test -race -v -count 1 ./...

.PHONY: simple-test
simple-test: test-tools
	test -z "$$(gofmt -s -l . | tee /dev/stderr)"
	staticcheck ./...
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
	-rm $(ROLES) $(PROTOC_OUTPUTS)
	$(MAKE) generate
	$(MAKE) manifests
	go mod tidy
	git diff --exit-code

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: manifests-ipam manifests-egress


.PHONY: manifests-egress
manifests-egress: $(CONTROLLER_GEN) $(ROLES) $(YQ)
	mkdir -p tmp/egress
	cp api/v2/egress_webhook.go api/v2/egress_types.go api/v2/groupversion_info.go tmp/egress
	$(CONTROLLER_GEN) $(CRD_OPTIONS) webhook paths="./tmp/egress/..." output:webhook:stdout output:crd:artifacts:config=config/crd/bases > config/webhook/egress/manifests.yaml
	sed -i 's/webhook-/egress-webhook-/g' config/webhook/egress/manifests.yaml
	# Reduce the size of Egress CRD by deleting `description` fields below the pod's template because it exceeds the limit of `metadata.annotations` length when it's applied with client-side mode.
	$(YQ) -i 'del(.spec.versions.[].schema.openAPIV3Schema.properties.spec.properties.template | ..  |select(key == "description"))' config/crd/bases/coil.cybozu.com_egresses.yaml
	rm -rf tmp 2> /dev/null

.PHONY: manifests-ipam
manifests-ipam: $(CONTROLLER_GEN) $(ROLES) $(YQ)
	mkdir -p tmp/ipam
	cp api/v2/addresspool_webhook.go api/v2/addresspool_types.go api/v2/addressblock_types.go api/v2/blockrequest_types.go api/v2/groupversion_info.go tmp/ipam
	$(CONTROLLER_GEN) $(CRD_OPTIONS) webhook paths="./tmp/ipam/..." output:webhook:stdout output:crd:artifacts:config=config/crd/bases > config/webhook/ipam/manifests.yaml
	sed -i 's/webhook-/ipam-webhook-/g' config/webhook/ipam/manifests.yaml
	rm -rf tmp 2> /dev/null


$(YQ):
	$(WGET) -O yq.tar.gz https://github.com/mikefarah/yq/releases/download/v$(YQ_VERSION)/yq_$(OS)_$(ARCH).tar.gz
	tar -C $(shell pwd)/bin/ -zxf yq.tar.gz ./yq_$(OS)_$(ARCH)
	mv $(shell pwd)/bin/yq_$(OS)_$(ARCH) $@
	rm -f yq.tar.gz
	chmod +x $@

COIL_IPAM_CONTROLLER_ROLE_DEPENDS = controllers/addresspool_controller.go \
	controllers/blockrequest_controller.go \
	pkg/ipam/pool.go \
	runners/garbage_collector.go

config/rbac/coil-ipam-controller_role.yaml: $(COIL_IPAM_CONTROLLER_ROLE_DEPENDS)
	-rm -rf work
	mkdir work
	sed '0,/^package/s/.*/package work/' controllers/addresspool_controller.go > work/addresspool_controller.go
	sed '0,/^package/s/.*/package work/' controllers/blockrequest_controller.go > work/blockrequest_controller.go
	sed '0,/^package/s/.*/package work/' pkg/ipam/pool.go > work/pool.go
	sed '0,/^package/s/.*/package work/' runners/garbage_collector.go > work/garbage_collector.go
	$(CONTROLLER_GEN) rbac:roleName=coil-ipam-controller paths=./work output:stdout > $@
	rm -rf work

COIL_IPAM_CONTROLLER_CERTS_ROLE_DEPENDS = controllers/addresspool_controller.go \
	controllers/blockrequest_controller.go \
	pkg/ipam/pool.go \
	runners/garbage_collector.go \
	pkg/cert/cert.go

config/rbac/coil-ipam-controller-certs_role.yaml: $(COIL_IPAM_CONTROLLER_CERTS_ROLE_DEPENDS)
	-rm -rf work
	mkdir work
	sed '0,/^package/s/.*/package work/' controllers/addresspool_controller.go > work/addresspool_controller.go
	sed '0,/^package/s/.*/package work/' controllers/blockrequest_controller.go > work/blockrequest_controller.go
	sed '0,/^package/s/.*/package work/' pkg/ipam/pool.go > work/pool.go
	sed '0,/^package/s/.*/package work/' runners/garbage_collector.go > work/garbage_collector.go
	sed '0,/^package/s/.*/package work/' pkg/cert/cert.go > work/cert.go
	$(CONTROLLER_GEN) rbac:roleName=coil-ipam-controller paths=./work output:stdout > $@
	rm -rf work

COIL_EGRESS_CONTROLLER_ROLE_DEPENDS = controllers/egress_controller.go \
	controllers/clusterrolebinding_controller.go

config/rbac/coil-egress-controller_role.yaml: $(COIL_EGRESS_CONTROLLER_ROLE_DEPENDS)
	-rm -rf work
	mkdir work
	sed '0,/^package/s/.*/package work/' controllers/egress_controller.go > work/egress_controller.go
	sed '0,/^package/s/.*/package work/' controllers/clusterrolebinding_controller.go > work/clusterrolebinding_controller.go
	$(CONTROLLER_GEN) rbac:roleName=coil-egress-controller paths=./work output:stdout > $@
	rm -rf work

COIL_EGRESS_CONTROLLER_CERTS_ROLE_DEPENDS = controllers/egress_controller.go \
	controllers/clusterrolebinding_controller.go \
	pkg/cert/cert.go

config/rbac/coil-egress-controller-certs_role.yaml: $(COIL_EGRESS_CONTROLLER_CERTS_ROLE_DEPENDS)
	-rm -rf work
	mkdir work
	sed '0,/^package/s/.*/package work/' controllers/egress_controller.go > work/egress_controller.go
	sed '0,/^package/s/.*/package work/' controllers/clusterrolebinding_controller.go > work/clusterrolebinding_controller.go
	sed '0,/^package/s/.*/package work/' pkg/cert/cert.go > work/cert.go
	$(CONTROLLER_GEN) rbac:roleName=coil-egress-controller paths=./work output:stdout > $@
	rm -rf work

COILD_DEPENDS = controllers/blockrequest_watcher.go \
	pkg/ipam/node.go \
	runners/coild_server.go

config/rbac/coild_role.yaml: $(COILD_DEPENDS)
	-rm -rf work
	mkdir work
	sed '0,/^package/s/.*/package work/' controllers/blockrequest_watcher.go > work/blockrequest_watcher.go
	sed '0,/^package/s/.*/package work/' controllers/egress_watcher.go > work/egress_watcher.go
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
certs: config/default/cert.pem config/default/key.pem config/default/ipam-cert.pem config/default/ipam-key.pem config/default/ipam/webhook_manifests_patch.yaml config/default/egress-cert.pem config/default/egress-key.pem config/default/egress/webhook_manifests_patch.yaml

config/default/cert.pem config/default/key.pem:
	go run ./cmd/gencert -outdir=$(PWD)/config/default -cn=coilv2-webhook-service -host=coilv2-webhook-service.kube-system.svc -certname=cert.pem -keyname=key.pem

config/default/ipam-cert.pem config/default/ipam-key.pem:
	go run ./cmd/gencert -outdir=$(PWD)/config/default -cn=coilv2-ipam-webhook-service -host=coilv2-ipam-webhook-service.kube-system.svc -certname=ipam-cert.pem -keyname=ipam-key.pem -ca=cert.pem -cakey=key.pem

config/default/egress-cert.pem config/default/egress-key.pem:
	go run ./cmd/gencert -outdir=$(PWD)/config/default -cn=coilv2-egress-webhook-service -host=coilv2-egress-webhook-service.kube-system.svc -certname=egress-cert.pem -keyname=egress-key.pem -ca=cert.pem -cakey=key.pem

config/default/ipam/webhook_manifests_patch.yaml: config/default/ipam-cert.pem config/default/ipam/webhook_manifests_patch.yaml.tmpl
	sed "s/%CACERT%/$$(base64 -w0 < $<)/g" $@.tmpl > $@

config/default/egress/webhook_manifests_patch.yaml: config/default/egress-cert.pem config/default/egress/webhook_manifests_patch.yaml.tmpl
	sed "s/%CACERT%/$$(base64 -w0 < $<)/g" $@.tmpl > $@

# Generate code
.PHONY: generate
generate: $(CONTROLLER_GEN)
	$(MAKE) $(PROTOC_OUTPUTS)
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

$(CONTROLLER_GEN):
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v$(CONTROLLER_TOOLS_VERSION))

pkg/cnirpc/cni.pb.go: pkg/cnirpc/cni.proto
	$(PROTOC) --go_out=module=github.com/cybozu-go/coil/v2:. $<

pkg/cnirpc/cni_grpc.pb.go: pkg/cnirpc/cni.proto
	$(PROTOC) --go-grpc_out=module=github.com/cybozu-go/coil/v2:. $<

../docs/cni-grpc.md: pkg/cnirpc/cni.proto
	$(PROTOC) --doc_out=../docs --doc_opt=markdown,$@ $<

.PHONY: build
build:
	GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o work/coil -ldflags="-s -w" cmd/coil/*.go
	GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o work/coil-ipam-controller -ldflags="-s -w" cmd/coil-ipam-controller/*.go
	GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o work/coil-egress-controller -ldflags="-s -w" cmd/coil-egress-controller/*.go
	GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o work/coil-egress -ldflags="-s -w" cmd/coil-egress/*.go
	GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o work/coil-installer -ldflags="-s -w" cmd/coil-installer/*.go
	GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o work/coil-router -ldflags="-s -w" cmd/coil-router/*.go
	GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o work/coild -ldflags="-s -w" cmd/coild/*.go

work/LICENSE:
	mkdir -p work
	cp ../LICENSE work

.PHONY: image
image: work/LICENSE
	docker buildx build --no-cache --load -t coil:dev .

.PHONY: push
push: work/LICENSE
	docker buildx build --no-cache --platform linux/amd64,linux/arm64/v8 --push -t ghcr.io/cybozu-go/coil:$(IMAGE_TAG) .

.PHONY: setup
setup:
	$(SUDO) apt-get update
	$(SUDO) apt-get -y install --no-install-recommends rsync unzip wget

	curl -sfL -o protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/protoc-$(PROTOC_VERSION)-linux-x86_64.zip
	unzip -o protoc.zip bin/protoc 'include/*'
	rm -f protoc.zip
	GOBIN=$(PWD)/bin go install google.golang.org/protobuf/cmd/protoc-gen-go@v$(PROTOC_GEN_GO_VERSION)
	GOBIN=$(PWD)/bin go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v$(PROTOC_GEN_GO_GRPC_VERSON)
	GOBIN=$(PWD)/bin go install github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@v$(PROTOC_GEN_DOC_VERSION)

.PHONY: setup-envtest
setup-envtest: ## Download setup-envtest locally if necessary
	# see https://github.com/kubernetes-sigs/controller-runtime/tree/master/tools/setup-envtest
	GOBIN=$(shell pwd)/bin go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
}
endef

.PHONY: test-tools
test-tools: staticcheck

.PHONY: staticcheck
staticcheck:
	if ! which staticcheck >/dev/null; then \
		env GOFLAGS= go install honnef.co/go/tools/cmd/staticcheck@latest; \
	fi

define comment_certs
    $(eval $@_FILE = $(1))
	@sed -i -E "{s/(^patchesStrategicMerge.*)/# \1/}" ${$@_FILE}
	@sed -i -E "{s/(^-.*webhook_manifests_patch.*)/# \1/}" ${$@_FILE}
endef

.PHONY: enable-certs-rotation
enable-certs-rotation:
	@sed -i "9,21 {s/^# //}" kustomization.yaml
	@sed -i -E 's/^(- coil-.*controller_role\.yaml)/# \1/g' config/rbac/kustomization.yaml
	@sed -i -E 's/^# (- coil-.*controller-certs_role\.yaml)/\1/g' config/rbac/kustomization.yaml
	@touch config/default/cert.pem
	@touch config/default/ipam-cert.pem
	@touch config/default/ipam-key.pem
	@touch config/default/egress-cert.pem
	@touch config/default/egress-key.pem
	@$(call comment_certs,"config/default/kustomization.yaml")
	@$(call comment_certs,"config/default/egress/v4/kustomization.yaml")
	@$(call comment_certs,"config/default/egress/v6/kustomization.yaml")
