# Makefile for coil

KUBEBUILDER_VERSION = 2.3.1
CONTROLLER_TOOLS_VERSION = v0.3.0

## DON'T EDIT BELOW THIS LINE
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

.PHONY: test check-generate manifests generate setup
