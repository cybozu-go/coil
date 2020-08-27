# Makefile for coil

KUBEBUILDER_VERSION = 2.3.1
K8S_VERSION=1.18.6
PROTOC_VERSION=3.12.4

## DON'T EDIT BELOW THIS LINE
SUDO=sudo
CONTROLLER_GEN := $(PWD)/bin/controller-gen
CRD_OPTIONS = "crd:crdVersions=v1"
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
GO111MODULE=on
KUBEBUILDER_ASSETS := $(PWD)/bin
PROTOC := PATH=$(PWD)/bin:$(PATH) $(PWD)/bin/protoc -I=$(PWD)/include:.
PODNSLIST = pod1 pod2 pod3
export GO111MODULE KUBEBUILDER_ASSETS PODNSLIST

test: test-tools
	go test -race -v -count 1 ./...
	go install ./...
	go vet ./...
	test -z $$(go fmt -s -l . | tee /dev/stderr)
	staticcheck ./...
	nilerr ./...

test-nodenet:
	go test -c ./pkg/nodenet
	for i in $@ $(PODNSLIST); do $(SUDO) ip netns delete $$i 2>/dev/null || true; done
	for i in $@ $(PODNSLIST); do $(SUDO) ip netns add $$i; done
	for i in $@ $(PODNSLIST); do $(SUDO) ip netns exec $$i ip link set lo up; done
	$(SUDO) ip netns exec $@ ./nodenet.test -test.v
	#for i in $@ $(PODNSLIST); do $(SUDO) ip netns delete $$i; done
	rm -f nodenet.test

check-generate:
	$(MAKE) manifests
	$(MAKE) generate
	git diff --exit-code --name-only 

# Generate manifests e.g. CRD, RBAC etc.
manifests: $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

	# workaround for CRD issue with k8s 1.18 & controller-gen 0.3
	# ref: https://github.com/kubernetes/kubernetes/issues/91395
	sed -i -r 's/^( +)description: Protocol for port\. Must be UDP, TCP, or SCTP\. Defaults to "TCP"\./\0\n\1default: TCP/' \
	  config/crd/bases/coil.cybozu.com_egresses.yaml

# Generate code
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

setup:
	$(SUDO) apt-get update
	$(SUDO) apt-get -y install --no-install-recommends rsync unzip

	mkdir -p bin
	curl -sfL https://go.kubebuilder.io/dl/$(KUBEBUILDER_VERSION)/$(GOOS)/$(GOARCH) | tar -xz -C /tmp/
	mv /tmp/kubebuilder_$(KUBEBUILDER_VERSION)_$(GOOS)_$(GOARCH)/bin/* bin/
	rm -rf /tmp/kubebuilder_*
	GOBIN=$(PWD)/bin go install sigs.k8s.io/controller-tools/cmd/controller-gen
	# we need kube-apiserver from k8s 1.18
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

test-tools: staticcheck nilerr

staticcheck:
	if ! which staticcheck >/dev/null; then \
		cd /tmp; env GOFLAGS= GO111MODULE=on go get honnef.co/go/tools/cmd/staticcheck; \
	fi

nilerr:
	if ! which nilerr >/dev/null; then \
		cd /tmp; env GOFLAGS= GO111MODULE=on go get github.com/gostaticanalysis/nilerr/cmd/nilerr; \
	fi

.PHONY: test test-tools test-nodenet staticcheck nilerr \
	check-generate manifests generate setup
