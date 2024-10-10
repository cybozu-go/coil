KIND_VERSION=0.23.0
KUBERNETES_VERSION=1.29.4
KUSTOMIZE_VERSION = 5.4.2
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

.PHONY: install-coil-egress-kindnet-v4
install-coil-egress-kindnet-v4: setup-nodes
	rm -rf tmp
	mkdir tmp 2> /dev/null
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	./kindnet-conf get 10-kindnet.conflist v4
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/kindnet/v4 | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	./kindnet-conf set 10-coil.conflist v4
	rm -rf tmp kindnet-conf

.PHONY: install-coil-egress-kindnet-v6
install-coil-egress-kindnet-v6: setup-nodes
	rm -rf tmp
	mkdir tmp 2> /dev/null
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	./kindnet-conf get 10-kindnet.conflist v6
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/kindnet/v6 | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	./kindnet-conf set 10-coil.conflist v6
	rm -rf tmp kindnet-conf

.PHONY: install-coil-egress-calico-v4
install-coil-egress-calico-v4: setup-nodes
	$(KUBECTL) create -f https://raw.githubusercontent.com/projectcalico/calico/v3.28.2/manifests/tigera-operator.yaml
	timeout 5m sh -c "until kubectl get -n tigera-operator deployment tigera-operator; do echo 'Waiting for tigera-operator to be deployed...'; sleep 1; done"
	$(KUBECTL) -n tigera-operator wait --timeout=5m --for=condition=available deployment/tigera-operator
	$(KUBECTL) apply -f configs/egress/calico/v4/calico-resources.yaml
	timeout 5m sh -c "until kubectl get -n calico-apiserver deployment calico-apiserver; do echo 'Waiting for Calico to be deployed...'; sleep 1; done"
	$(KUBECTL) -n calico-apiserver wait --timeout=5m --for=condition=available deployment/calico-apiserver
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/calico/v4 | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m


.PHONY: install-coil-egress-calico-v6
install-coil-egress-calico-v6: setup-nodes
	$(KUBECTL) create -f https://raw.githubusercontent.com/projectcalico/calico/v3.28.2/manifests/tigera-operator.yaml
	timeout 5m sh -c "until kubectl get -n tigera-operator deployment tigera-operator; do echo 'Waiting for tigera-operator to be deployed...'; sleep 1; done"
	$(KUBECTL) -n tigera-operator wait --timeout=5m --for=condition=available deployment/tigera-operator
	$(KUBECTL) apply -f configs/egress/calico/v6/calico-resources.yaml
	timeout 5m sh -c "until kubectl get -n calico-apiserver deployment calico-apiserver; do echo 'Waiting for Calico to be deployed...'; sleep 1; done"
	$(KUBECTL) -n calico-apiserver wait --timeout=5m --for=condition=available deployment/calico-apiserver
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/calico/v6 | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m

.PHONY: setup-nodes
setup-nodes:
	$(KUBECTL) taint nodes coil-worker test:NoSchedule
	$(KUBECTL) taint nodes coil-worker2 test:NoSchedule
	$(KUBECTL) label nodes coil-worker test=coil
	$(KUBECTL) label nodes coil-worker2 test=coil

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
