include ../common.mk

IMAGE = kindest/node:v1.19.11
KIND_VERSION=0.11.1
KUSTOMIZE_VERSION = 4.1.3
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
	curl -o $(BINDIR)/kubectl -sfL https://storage.googleapis.com/kubernetes-release/release/$(subst kindest/node:,,$(IMAGE))/bin/linux/amd64/kubectl
	chmod a+x $(BINDIR)/kubectl
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
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone . | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-controller

.PHONY: test
test:
	go build -o echotest ./echo-server
	docker cp echotest coil-control-plane:/usr/local/bin
	rm echotest
	go test -count 1 -v . -args -ginkgo.progress -ginkgo.v

.PHONY: logs
logs:
	rm -rf logs.tar.gz logs
	$(KIND) export logs --name=coil ./logs
	tar czf logs.tar.gz logs
	rm -rf logs

.PHONY: setup
setup:
	mkdir -p $(BINDIR)
	curl -o $(BINDIR)/kind -sfL https://kind.sigs.k8s.io/dl/v$(KIND_VERSION)/kind-linux-amd64
	chmod a+x $(BINDIR)/kind
	curl -sfL https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv$(KUSTOMIZE_VERSION)/kustomize_v$(KUSTOMIZE_VERSION)_linux_amd64.tar.gz | tar -xz -C $(BINDIR)
