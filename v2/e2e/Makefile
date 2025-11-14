KIND_VERSION=0.29.0
KUBERNETES_VERSION=1.32.5
KUSTOMIZE_VERSION = 5.6.0
BINDIR := $(abspath $(PWD)/../bin)

ARCH := $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
KIND := $(BINDIR)/kind
KUBECTL := $(BINDIR)/kubectl
KUSTOMIZE := $(BINDIR)/kustomize
export KUBECTL

KIND_CONFIG = kind-config.yaml
ifeq ($(TEST_IPV6),true)
	ifeq ($(TEST_IPV4),true)
		ifeq ($(IPV6_PRIMARY),true)
			ifeq ($(WITH_KINDNET),true)
				KIND_CONFIG = kind-config_dualstack_v6_kindnet.yaml
			else
				KIND_CONFIG = kind-config_dualstack_v6.yaml
			endif
		else
			ifeq ($(WITH_KINDNET),true)
				KIND_CONFIG = kind-config_dualstack_kindnet.yaml
			else
				KIND_CONFIG = kind-config_dualstack.yaml
			endif
		endif
	else
		ifeq ($(WITH_KINDNET),true)
			KIND_CONFIG = kind-config_kindnet_v6.yaml
		else
			KIND_CONFIG = kind-config_v6.yaml
		endif
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

.PHONY: install-coil-originatingonly
install-coil-originatingonly: setup-nodes
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/originatingonly  | $(KUBECTL) apply -f -
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
	./kindnet-conf --action set --cni-config 10-coil.conflist
	rm -rf tmp kindnet-conf

.PHONY: install-coil-egress-v4-originatingonly
install-coil-egress-v4-originatingonly: setup-nodes
	rm -rf tmp
	mkdir tmp 2> /dev/null
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	$(KUBECTL) rollout status daemonset kindnet -n kube-system --timeout 120s
	./kindnet-conf --action get
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/v4-originatingonly | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	./kindnet-conf --action set --cni-config 10-coil.conflist
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
	./kindnet-conf --action set --cni-config 10-coil.conflist --protocol v6
	rm -rf tmp kindnet-conf

.PHONY: install-coil-egress-v6-originatingonly
install-coil-egress-v6-originatingonly: setup-nodes
	rm -rf tmp
	mkdir tmp 2> /dev/null
	$(KUBECTL) rollout status daemonset kindnet -n kube-system --timeout 120s
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	./kindnet-conf --action get --protocol v6
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/v6-originatingonly | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	./kindnet-conf --action set --cni-config 10-coil.conflist --protocol v6
	rm -rf tmp kindnet-conf

.PHONY: install-coil-egress-v4-nft
install-coil-egress-v4-nft: setup-nodes
	rm -rf tmp
	mkdir tmp 2> /dev/null
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	$(KUBECTL) rollout status daemonset kindnet -n kube-system --timeout 120s
	./kindnet-conf --action get
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/v4-nft | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	./kindnet-conf --action set --cni-config 10-coil.conflist
	rm -rf tmp kindnet-conf

.PHONY: install-coil-egress-v4-nft-originatingonly
install-coil-egress-v4-nft-originatingonly: setup-nodes
	rm -rf tmp
	mkdir tmp 2> /dev/null
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	$(KUBECTL) rollout status daemonset kindnet -n kube-system --timeout 120s
	./kindnet-conf --action get
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/v4-nft-originatingonly | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	./kindnet-conf --action set --cni-config 10-coil.conflist
	rm -rf tmp kindnet-conf

.PHONY: install-coil-egress-v6-nft
install-coil-egress-v6-nft: setup-nodes
	rm -rf tmp
	mkdir tmp 2> /dev/null
	$(KUBECTL) rollout status daemonset kindnet -n kube-system --timeout 120s
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	./kindnet-conf --action get --protocol v6
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/v6-nft | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	./kindnet-conf --action set --cni-config 10-coil.conflist --protocol v6
	rm -rf tmp kindnet-conf

.PHONY: install-coil-egress-v6-nft-originatingonly
install-coil-egress-v6-nft-originatingonly: setup-nodes
	rm -rf tmp
	mkdir tmp 2> /dev/null
	$(KUBECTL) rollout status daemonset kindnet -n kube-system --timeout 120s
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	./kindnet-conf --action get --protocol v6
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/v6-nft-originatingonly | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	./kindnet-conf --action set --cni-config 10-coil.conflist --protocol v6
	rm -rf tmp kindnet-conf

.PHONY: install-coil-egress-dualstack
install-coil-egress-dualstack: setup-nodes
	rm -rf tmp
	mkdir tmp 2> /dev/null
	$(KUBECTL) rollout status daemonset kindnet -n kube-system --timeout 120s
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	./kindnet-conf --action get --protocol v4
	./kindnet-conf --action get --protocol v6
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/dualstack | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	./kindnet-conf --action set --cni-config 10-coil.conflist --protocol v4
	./kindnet-conf --action set --cni-config 10-coil.conflist --protocol v6
	rm -rf tmp kindnet-conf

.PHONY: install-coil-egress-dualstack-originatingonly
install-coil-egress-dualstack-originatingonly: setup-nodes
	rm -rf tmp
	mkdir tmp 2> /dev/null
	$(KUBECTL) rollout status daemonset kindnet -n kube-system --timeout 120s
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	./kindnet-conf --action get --protocol v4
	./kindnet-conf --action get --protocol v6
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/dualstack-originatingonly | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	./kindnet-conf --action set --cni-config 10-coil.conflist --protocol v4
	./kindnet-conf --action set --cni-config 10-coil.conflist --protocol v6
	rm -rf tmp kindnet-conf

.PHONY: install-coil-egress-dualstack-nft
install-coil-egress-dualstack-nft: setup-nodes
	rm -rf tmp
	mkdir tmp 2> /dev/null
	$(KUBECTL) rollout status daemonset kindnet -n kube-system --timeout 120s
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	./kindnet-conf --action get --protocol v4
	./kindnet-conf --action get --protocol v6
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/dualstack-nft | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	./kindnet-conf --action set --cni-config 10-coil.conflist --protocol v4
	./kindnet-conf --action set --cni-config 10-coil.conflist --protocol v6
	rm -rf tmp kindnet-conf

.PHONY: install-coil-egress-dualstack-nft-originatingonly 
install-coil-egress-dualstack-nft-originatingonly: setup-nodes
	rm -rf tmp
	mkdir tmp 2> /dev/null
	$(KUBECTL) rollout status daemonset kindnet -n kube-system --timeout 120s
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	./kindnet-conf --action get --protocol v4
	./kindnet-conf --action get --protocol v6
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone configs/egress/dualstack-nft-originatingonly  | $(KUBECTL) apply -f -
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	./kindnet-conf --action set --cni-config 10-coil.conflist --protocol v4
	./kindnet-conf --action set --cni-config 10-coil.conflist --protocol v6
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
test: setup-echotest setup-echotest-image
	TEST_IPAM=$(TEST_IPAM) TEST_EGRESS=$(TEST_EGRESS) go test -count 1 -v . -args -ginkgo.progress -ginkgo.v

.PHONY: build-echotest
build-echotest:
	CGO_ENABLED=0 GOARCH=$(ARCH) GOOS=linux go build -o echotest ./echo-server

.PHONY: setup-echotest
setup-echotest: build-echotest
	docker cp echotest coil-control-plane:/usr/local/bin
	rm echotest

.PHONY: build-echotest-image
build-echotest-image:
	TARGETARCH=$(ARCH) docker build -t echotest:dev -f echo-server/Dockerfile echo-server/.

.PHONY: setup-echotest-image
setup-echotest-image: build-echotest-image
	$(KIND) load docker-image echotest:dev --name coil

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
endef

.PHONY: enable-certs-rotation
enable-certs-rotation:
	@sed -i "9,21 {s/^# //}" kustomization.yaml
	@sed -i "9,21 {s/^# //}" configs/originatingonly/kustomization.yaml
	@sed -i "18,24 {s/^# //}" configs/egress/v4/kustomization.yaml
	@sed -i "18,24 {s/^# //}" configs/egress/v6/kustomization.yaml
	@sed -i "18,24 {s/^# //}" configs/egress/dualstack/kustomization.yaml
	@sed -i -E 's/^(- coil-.*controller_role\.yaml)/# \1/g' ../config/rbac/kustomization.yaml
	@sed -i -E 's/^# (- coil-.*controller-certs_role\.yaml)/\1/g' ../config/rbac/kustomization.yaml
	@sed -i -E 's/^(- \.\.\/coil-.*controller_role\.yaml)/# \1/g' ../config/rbac/egress/kustomization.yaml
	@sed -i -E 's/^# (- \.\.\/coil-.*controller-certs_role\.yaml)/\1/g' ../config/rbac/egress/kustomization.yaml
	@touch ../config/default/cert.pem
	@touch ../config/default/ipam-cert.pem
	@touch ../config/default/ipam-key.pem
	@touch ../config/default/egress-cert.pem
	@touch ../config/default/egress-key.pem
	@$(call comment_certs,"../config/default/kustomization.yaml")
	@$(call comment_certs,"../config/default/egress/v4/kustomization.yaml")
	@$(call comment_certs,"../config/default/egress/v6/kustomization.yaml")
	@$(call comment_certs,"../config/default/egress/v4-nft/kustomization.yaml")
	@$(call comment_certs,"../config/default/egress/v6-nft/kustomization.yaml")
	@$(call comment_certs,"../config/default/egress/v4-originatingonly/kustomization.yaml")
	@$(call comment_certs,"../config/default/egress/v6-originatingonly/kustomization.yaml")
	@$(call comment_certs,"../config/default/egress/v4-nft-originatingonly/kustomization.yaml")
	@$(call comment_certs,"../config/default/egress/v6-nft-originatingonly/kustomization.yaml")

$(KIND):
	mkdir -p $(dir $@)
	curl -sfL -o $@ https://github.com/kubernetes-sigs/kind/releases/download/v$(KIND_VERSION)/kind-$(OS)-$(ARCH)
	chmod a+x $@

$(KUBECTL):
	mkdir -p $(dir $@)
	curl -sfL -o $@ https://dl.k8s.io/release/v$(KUBERNETES_VERSION)/bin/$(OS)/$(ARCH)/kubectl
	chmod a+x $@

$(KUSTOMIZE):
	mkdir -p $(dir $@)
	curl -sfL https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv$(KUSTOMIZE_VERSION)/kustomize_v$(KUSTOMIZE_VERSION)_$(OS)_$(ARCH).tar.gz | tar -xz -C $(BINDIR)
	chmod a+x $@
