KIND_VERSION=0.19.0
KUBERNETES_VERSION=1.27.1
KUSTOMIZE_VERSION = 5.0.1
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
start: $(KIND) $(KUBECTL) $(KUSTOMIZE)
	$(KIND) create cluster --image kindest/node:v$(KUBERNETES_VERSION) --name coil --config $(KIND_CONFIG)

.PHONY: stop
stop: $(KIND)
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
	CGO_ENABLED=0 go build -o echotest ./echo-server
	docker cp echotest coil-control-plane:/usr/local/bin
	rm echotest
	go test -count 1 -v . -args -ginkgo.progress -ginkgo.v

.PHONY: logs
logs:
	rm -rf logs.tar.gz logs
	$(KIND) export logs --name=coil ./logs
	tar czf logs.tar.gz logs
	rm -rf logs

$(KIND):
	mkdir -p $(dir $@)
	curl -sfL -o $@ https://github.com/kubernetes-sigs/kind/releases/download/v$(KIND_VERSION)/kind-linux-amd64
	chmod a+x $@

$(KUBECTL):
	mkdir -p $(dir $@)
	curl -sfL -o $@ https://dl.k8s.io/release/v$(KUBERNETES_VERSION)/bin/linux/amd64/kubectl
	chmod a+x $@

$(KUSTOMIZE):
	mkdir -p $(dir $@)
	curl -sfL https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv$(KUSTOMIZE_VERSION)/kustomize_v$(KUSTOMIZE_VERSION)_linux_amd64.tar.gz | tar -xz -C $(BINDIR)
	chmod a+x $@
