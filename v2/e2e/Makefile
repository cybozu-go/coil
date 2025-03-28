KIND_VERSION=0.27.0
KUBERNETES_VERSION=1.31.6
KUSTOMIZE_VERSION = 5.6.0
BINDIR := $(abspath $(PWD)/../bin)

KIND := $(BINDIR)/kind
KUBECTL := $(BINDIR)/kubectl
KUSTOMIZE := $(BINDIR)/kustomize
export KUBECTL

KIND_CONFIG = kind-config.yaml
ifeq ($(TEST_IPV6),true)
	ifeq ($(WITH_KINDNET),true)
		KIND_CONFIG = kind-config_kindnet_v6.yaml
	else 
		KIND_CONFIG = kind-config_v6.yaml
	endif
else
	ifeq ($(WITH_KINDNET),true)
		KIND_CONFIG = kind-config_kindnet.yaml
	endif
endif

TIMEOUT=0

.PHONY: start
start: $(KIND) $(KUBECTL) $(KUSTOMIZE)
	$(KIND) create cluster --image kindest/node:v$(KUBERNETES_VERSION) --name coil --config $(KIND_CONFIG)

.PHONY: stop
stop: $(KIND)
	$(KIND) delete cluster --name coil

.PHONY: install-coil
install-coil: setup-nodes
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone . | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-ipam-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller

.PHONY: install-coil-egress-v4
install-coil-egress-v4: setup-nodes
	rm -rf tmp
	mkdir tmp 2> /dev/null
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	$(KUBECTL) rollout status daemonset kindnet -n kube-system --timeout 120s
	./kindnet-conf --action get
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/v4 | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	./kindnet-conf --action set --file 10-coil.conflist
	rm -rf tmp kindnet-conf

.PHONY: install-coil-egress-v6
install-coil-egress-v6: setup-nodes
	rm -rf tmp
	mkdir tmp 2> /dev/null
	$(KUBECTL) rollout status daemonset kindnet -n kube-system --timeout 120s
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	./kindnet-conf --action get --protocol v6
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/v6 | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	./kindnet-conf --action set --file 10-coil.conflist --protocol v6
	rm -rf tmp kindnet-conf

.PHONY: setup-nodes
setup-nodes:
	$(KUBECTL) taint nodes coil-worker test:NoSchedule
	$(KUBECTL) taint nodes coil-worker2 test:NoSchedule
	$(KUBECTL) label nodes coil-worker test=coil
	$(KUBECTL) label nodes coil-worker2 test=coil

TEST_IPAM ?= true
TEST_EGRESS ?= true
.PHONY: test
test: setup-echotest
	TEST_IPAM=$(TEST_IPAM) TEST_EGRESS=$(TEST_EGRESS) go test -count 1 -v . -args -ginkgo.progress -ginkgo.v

.PHONY: test-ipam
test-ipam: setup-echotest
	TEST_IPAM=$(TEST_IPAM) TEST_EGRESS=$(TEST_EGRESS) go test -count 1 -v . -args -ginkgo.progress -ginkgo.v

.PHONY: test-egress
test-egress: setup-echotest
	TEST_IPAM=$(TEST_IPAM) TEST_EGRESS=$(TEST_EGRESS) go test -count 1 -v . -args -ginkgo.progress -ginkgo.v

.PHONY: setup-echotest
setup-echotest:
	CGO_ENABLED=0 go build -o echotest ./echo-server
	docker cp echotest coil-control-plane:/usr/local/bin
	rm echotest

.PHONY: logs
logs:
	rm -rf logs.tar.gz logs
	$(KIND) export logs --name=coil ./logs
	tar czf logs.tar.gz logs
	rm -rf logs

define comment_certs
    $(eval $@_FILE = $(1))
	@sed -i -E "{s/(^patchesStrategicMerge.*)/# \1/}" ${$@_FILE}
	@sed -i -E "{s/(^-.*webhook_manifests_patch.*)/# \1/}" ${$@_FILE}
	@sed -i -E "{s/(^ +files.*)/# \1/g}" ${$@_FILE}
	@sed -i -E "{s/(^ +.*\.pem.*)/# \1/g}" ${$@_FILE}
	@sed -i -E "{s/(^ +.*\/tls.*)/# \1/g}" ${$@_FILE}
endef    

.PHONY: enable-certs-generation
enable-certs-generation:
	@sed -i "9,21 {s/^# //}" kustomization.yaml
	@sed -i -E 's/^(- coil-.*controller_role\.yaml)/# \1/g' ../config/rbac/kustomization.yaml
	@sed -i -E 's/^# (- coil-.*controller-certs_role\.yaml)/\1/g' ../config/rbac/kustomization.yaml
	@$(call comment_certs,"../config/default/kustomization.yaml")
	@$(call comment_certs,"../config/default/egress/v4/kustomization.yaml")
	@$(call comment_certs,"../config/default/egress/v6/kustomization.yaml")

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
