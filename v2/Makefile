# Makefile for coil

KUBEBUILDER_VERSION = 2.3.1
CONTROLLER_TOOLS_VERSION = v0.3.0
K8S_VERSION=1.18.6

## DON'T EDIT BELOW THIS LINE
SUDO=sudo
CONTROLLER_GEN := $(PWD)/bin/controller-gen
CRD_OPTIONS = "crd:crdVersions=v1"
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
GO111MODULE=on
KUBEBUILDER_ASSETS := $(PWD)/bin
export GO111MODULE KUBEBUILDER_ASSETS

test:
	go test -race -v -count 1 ./...
	go install ./...

check-generate:
	$(MAKE) manifests
	$(MAKE) generate
	git diff --exit-code --name-only 

# Generate manifests e.g. CRD, RBAC etc.
manifests: $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

	# workaround for CRD issue with k8s 1.18 & controller-gen 0.3
	# ref: https://github.com/kubernetes/kubernetes/issues/91395
	sed -i -r 's/^( +)  or SCTP\. Defaults to "TCP"\./\0\n\1default: TCP/' \
	  config/crd/bases/coil.cybozu.com_egresses.yaml

# Generate code
generate: $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

 $(CONTROLLER_GEN):
	$(MAKE) setup

setup:
	mkdir -p bin
	curl -sfL https://go.kubebuilder.io/dl/$(KUBEBUILDER_VERSION)/$(GOOS)/$(GOARCH) | tar -xz -C /tmp/
	mv /tmp/kubebuilder_$(KUBEBUILDER_VERSION)_$(GOOS)_$(GOARCH)/bin/* bin/
	rm -rf /tmp/kubebuilder_*
	cd /tmp; GOBIN=$(PWD)/bin go get sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)
	# we need kube-apiserver from k8s 1.18
	$(SUDO) apt-get update
	$(SUDO) apt-get -y install --no-install-recommends rsync
	rm -rf tmp && mkdir -p tmp
	curl -sfL https://github.com/kubernetes/kubernetes/archive/v$(K8S_VERSION).tar.gz | tar zxf - -C tmp
	mv tmp/kubernetes-$(K8S_VERSION) tmp/kubernetes
	cd tmp/kubernetes; make all WHAT="cmd/kube-apiserver"
	mv tmp/kubernetes/_output/bin/kube-apiserver bin/
	rm -rf tmp

.PHONY: test check-generate manifests generate setup
