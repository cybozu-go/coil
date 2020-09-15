include ../common.mk

IMAGE = kindest/node:v1.18.8
KIND_VERSION=0.9.0
BINDIR := $(abspath $(PWD)/../bin)

KIND := $(BINDIR)/kind
KUBECTL := $(BINDIR)/kubectl
KUSTOMIZE := $(BINDIR)/kustomize
export KUBECTL

KIND_CONFIG = kind-config.yaml
ifeq ($(TEST_IPV6),true)
KIND_CONFIG = kind-config_v6.yaml
endif

.PHONY: start
start:
	$(KIND) create cluster --image $(IMAGE) --name coil --config $(KIND_CONFIG)

.PHONY: stop
stop:
	$(KIND) delete cluster --name coil

.PHONY: install-coil
install-coil:
	$(KUBECTL) taint nodes coil-worker test:NoSchedule
	$(KUBECTL) taint nodes coil-worker2 test:NoSchedule
	$(KUBECTL) label nodes coil-worker test=coil
	$(KUBECTL) label nodes coil-worker2 test=coil
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) --load_restrictor=none build . | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-controller

.PHONY: test
test:
	go test -count 1 -v . -args -ginkgo.progress -ginkgo.v

.PHONY: setup
setup:
	mkdir -p $(BINDIR)
	curl -o $(BINDIR)/kind -sfL https://kind.sigs.k8s.io/dl/v$(KIND_VERSION)/kind-linux-amd64
	curl -o $(BINDIR)/kubectl -sfL https://storage.googleapis.com/kubernetes-release/release/v$(K8S_VERSION)/bin/linux/amd64/kubectl
	curl -sfL https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv3.7.0/kustomize_v3.7.0_linux_amd64.tar.gz | tar -xz -C $(BINDIR)
	chmod a+x $(BINDIR)/kubectl $(BINDIR)/kind
