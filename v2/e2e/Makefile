KIND_VERSION=0.31.0
KUBERNETES_VERSION ?= 1.35.1
KUSTOMIZE_VERSION = 5.8.1
BINDIR := $(abspath $(PWD)/../bin)

ARCH := $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
KIND := $(BINDIR)/kind
KUBECTL := $(BINDIR)/kubectl
KUSTOMIZE := $(BINDIR)/kustomize
export KUBECTL

# E2E scenario inputs (override via environment or `make X=Y`).
WITH_IPAM ?= true
TEST_IPV4 ?= true
TEST_IPV6 ?= false
IPV6_PRIMARY ?= false
BACKEND ?= iptables
ORIGINATINGONLY ?= false
TEST_EGRESS ?= true
# IPAM mode uses coil as the CNI; egress-only mode keeps kindnet as the CNI.
WITH_KINDNET ?= $(if $(filter true,$(WITH_IPAM)),false,true)

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

TIMEOUT = 0

.PHONY: start
start: $(KIND) $(KUBECTL) $(KUSTOMIZE)
	$(KIND) create cluster --image kindest/node:v$(KUBERNETES_VERSION) --name coil --config $(KIND_CONFIG)

.PHONY: stop
stop: $(KIND)
	$(KIND) delete cluster --name coil

# Egress mode kustomize overlay selection.
EGRESS_PROTO := $(if $(filter true,$(TEST_IPV4)),$(if $(filter true,$(TEST_IPV6)),dualstack,v4),v6)
EGRESS_NFT_SUFFIX := $(if $(filter nftables,$(BACKEND)),-nft,)
EGRESS_OO_SUFFIX := $(if $(filter true,$(ORIGINATINGONLY)),-originatingonly,)

# Protocols passed to kindnet-configurer (only used in egress mode).
KINDNET_PROTOS := $(strip \
  $(if $(filter true,$(TEST_IPV4)),v4,) \
  $(if $(filter true,$(TEST_IPV6)),v6,))

ifeq ($(WITH_IPAM),true)
  KUSTOMIZE_DIR := $(if $(filter true,$(ORIGINATINGONLY)),configs/originatingonly,.)
else
  KUSTOMIZE_DIR := configs/egress/$(EGRESS_PROTO)$(EGRESS_NFT_SUFFIX)$(EGRESS_OO_SUFFIX)
endif

# Unified install target. Behavior depends on WITH_IPAM:
#   true  -> coil acts as the CNI; deploy IPAM + egress controllers.
#   false -> coil runs as standalone egress; rewrite kindnet's CNI config to
#            chain coil after kindnet, then deploy egress controllers + coild.
.PHONY: install-coil
install-coil: setup-nodes
	$(KIND) load docker-image --name coil coil:dev
ifeq ($(WITH_IPAM),false)
	@if [ -z "$(strip $(KINDNET_PROTOS))" ]; then \
		echo "ERROR: WITH_IPAM=false requires TEST_IPV4=true and/or TEST_IPV6=true"; \
		exit 1; \
	fi
	rm -rf tmp && mkdir tmp 2> /dev/null
	CGO_ENABLED=0 go build -o kindnet-conf ./kindnet-configurer
	$(KUBECTL) rollout status daemonset kindnet -n kube-system --timeout 120s
	$(foreach p,$(KINDNET_PROTOS),./kindnet-conf --action get --protocol $(p) &&) :
endif
	$(KUSTOMIZE) build --load-restrictor=LoadRestrictionsNone $(KUSTOMIZE_DIR) | $(KUBECTL) apply -f -
ifeq ($(WITH_IPAM),true)
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-ipam-controller
endif
	$(KUBECTL) -n kube-system wait --timeout=3m --for=condition=available deployment/coil-egress-controller
ifeq ($(WITH_IPAM),false)
	$(KUBECTL) rollout status daemonset coild -n kube-system --timeout=3m
	$(foreach p,$(KINDNET_PROTOS),./kindnet-conf --action set --cni-config 10-coil.conflist --protocol $(p) &&) :
	rm -rf tmp kindnet-conf
endif

# CI-only: enable Docker IPv6 mode when TEST_IPV6=true. Requires sudo.
.PHONY: ci-enable-docker-ipv6
ci-enable-docker-ipv6:
ifeq ($(TEST_IPV6),true)
	sudo mkdir -p /etc/docker
	sudo cp daemon.json /etc/docker/daemon.json
	sudo systemctl restart docker.service
	sleep 10
endif

.PHONY: setup-nodes
setup-nodes:
	$(KUBECTL) taint nodes coil-worker test:NoSchedule
	$(KUBECTL) taint nodes coil-worker2 test:NoSchedule
	$(KUBECTL) label nodes coil-worker test=coil
	$(KUBECTL) label nodes coil-worker2 test=coil

TEST_IPAM ?= $(WITH_IPAM)
TEST_ORIGINATINGONLY ?= $(ORIGINATINGONLY)
.PHONY: test
test: setup-echotest setup-echotest-image
	TEST_IPAM=$(TEST_IPAM) TEST_EGRESS=$(TEST_EGRESS) \
	  TEST_IPV4=$(TEST_IPV4) TEST_IPV6=$(TEST_IPV6) \
	  TEST_ORIGINATINGONLY=$(TEST_ORIGINATINGONLY) \
	  go test -count 1 -v . -args -ginkgo.progress -ginkgo.v

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
