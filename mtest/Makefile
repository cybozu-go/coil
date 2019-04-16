# Makefile for multi-host testing using ginkgo

# configuration variables
BRIDGE_ADDRESS = 10.0.0.1
## 'HOST' runs CKE and management etcd
HOST1 = 10.0.0.11
## 'NODE' runs etcd and k8s, managed by CKE
NODE1 = 10.0.0.101
NODE2 = 10.0.0.102
CONTAINER_RUNTIME = remote
ifeq ($(CONTAINER_RUNTIME),remote)
RUNTIME_ENDPOINT = unix:///var/run/k8s-containerd.sock
endif
PLACEMAT = /usr/bin/placemat
GINKGO = env GOFLAGS=-mod=vendor $(GOPATH)/bin/ginkgo
MANAGEMENT_ETCD_VERSION = 3.3.11
VAULT_VERSION = 1.0.2
K8S_VERSION = 1.13.4
PLACEMAT_DATADIR = /var/scratch/placemat
VM_SHARED_DIR = /mnt/placemat
CONTAINERD_VERSION = 1.2.4
PROTOBUF_VERSION = 3.7.0
PROTOC = /usr/local/bin/protoc
CRITOOLS_VERSION = 1.13.0
CT_VERSION = 0.9.0
CT = /usr/local/bin/ct
SUDO = sudo
PACKAGES = libseccomp-dev automake autoconf libtool
SUITE = ./functions
ifeq ($(findstring /,$(SUITE)),)
  SUITE_PACKAGE = ./$(SUITE)
else
  SUITE_PACKAGE = $(SUITE)
endif

export BRIDGE_ADDRESS HOST1 NODE1 NODE2 PLACEMAT GINKGO CONTAINER_RUNTIME SUITE_PACKAGE

# non-configuration variables
MANAGEMENT_ETCD_ARCHIVE := etcd-v$(MANAGEMENT_ETCD_VERSION)-linux-amd64.tar.gz
VAULT_ARCHIVE := vault_$(VAULT_VERSION)_linux_amd64.zip
KUBECTL_URL := https://storage.googleapis.com/kubernetes-release/release/v$(K8S_VERSION)/bin/linux/amd64/kubectl
SSH_PRIVKEY = $(realpath ./mtest_key)
OUTPUT := ./output
CKECONFIG := $(abspath $(OUTPUT))/cke.yml
COILCTL := $(abspath $(OUTPUT))/coilctl
COILIMAGE := $(abspath $(OUTPUT))/coil.img
COREOS_IMAGE := coreos_production_qemu_image.img
KUBECTL := $(abspath $(OUTPUT))/kubectl
DEPLOYYAML := $(abspath $(OUTPUT))/deploy.yml

export SSH_PRIVKEY CKECONFIG COILCTL COILIMAGE KUBECTL DEPLOYYAML

CONTAINERD_FILES = \
	$(OUTPUT)/containerd \
	$(OUTPUT)/containerd-shim \
	$(OUTPUT)/ctr

TEST_DEPS = \
	$(OUTPUT)/coilctl \
	$(OUTPUT)/coil.img

PLACEMAT_DEPS = \
	$(CONTAINERD_FILES) \
	$(COREOS_IMAGE) \
	$(OUTPUT)/bird-node1.conf \
	$(OUTPUT)/bird-node2.conf \
	$(OUTPUT)/cke-cluster.yml \
	$(OUTPUT)/cke.yml \
	$(OUTPUT)/cluster.yml \
	$(OUTPUT)/crictl \
	$(OUTPUT)/deploy.yml \
	$(OUTPUT)/etcd \
	$(OUTPUT)/etcdctl \
	$(OUTPUT)/host1.ign \
	$(OUTPUT)/kubectl \
	$(OUTPUT)/mtest_key \
	$(OUTPUT)/node1.ign \
	$(OUTPUT)/node2.ign \
	$(OUTPUT)/setup-cke.sh \
	$(OUTPUT)/setup-coil.sh \
	$(OUTPUT)/vault

all:
	@echo "Specify one of these targets:"
	@echo
	@echo "    placemat - run VMs in placemat."
	@echo "    stop     - stop placemat."
	@echo "    test     - run test in placemat env."
	@echo "    setup    - install dependencies."
	@echo
	@echo "'make test' accepts SUITE."

# TEST_DEPS
$(OUTPUT)/coil.img:
	rm -rf tmpbin
	mkdir tmpbin
	cd ..; GOBIN=$(shell pwd)/tmpbin CGO_ENABLED=0 go install -mod=vendor ./pkg/hypercoil
	ln -s hypercoil tmpbin/coil
	ln -s hypercoil tmpbin/coild
	ln -s hypercoil tmpbin/coil-controller
	ln -s hypercoil tmpbin/coilctl
	ln -s hypercoil tmpbin/coil-installer
	sudo podman build --no-cache --rm=false -f Dockerfile -t quay.io/cybozu/coil:dev tmpbin
	mkdir -p $(OUTPUT)
	sudo podman save -o $@ quay.io/cybozu/coil:dev

$(OUTPUT)/coilctl: $(OUTPUT)/coil.img
	cp tmpbin/coilctl $@

# PLACEMAT_DEPS
$(MANAGEMENT_ETCD_ARCHIVE):
	curl -sSLf -o $@ https://github.com/coreos/etcd/releases/download/v$(MANAGEMENT_ETCD_VERSION)/etcd-v$(MANAGEMENT_ETCD_VERSION)-linux-amd64.tar.gz

$(VAULT_ARCHIVE):
	curl -sSLf -o $@ https://releases.hashicorp.com/vault/$(VAULT_VERSION)/$(VAULT_ARCHIVE)

$(COREOS_IMAGE):
	curl -sSLf -o $@.bz2 https://stable.release.core-os.net/amd64-usr/current/$@.bz2
	bunzip2 -q $@.bz2

$(OUTPUT)/etcd $(OUTPUT)/etcdctl: $(MANAGEMENT_ETCD_ARCHIVE)
	mkdir -p $(OUTPUT)
	tar -C $(OUTPUT) -x -z --wildcards --strip-components 1 -f $< '*/$(notdir $@)'
	touch $@

$(OUTPUT)/vault: $(VAULT_ARCHIVE)
	mkdir -p $(OUTPUT)
	unzip $< -d $(OUTPUT)
	touch $@

$(OUTPUT)/kubectl:
	mkdir -p $(OUTPUT)
	curl -o $@ -Lf $(KUBECTL_URL)
	chmod a+x $@

$(OUTPUT)/setup-cke.sh: setup-cke.sh
	mkdir -p $(OUTPUT)
	cp $< $@
	chmod a+x $@

$(OUTPUT)/setup-coil.sh: setup-coil.sh
	mkdir -p $(OUTPUT)
	cp $< $@
	chmod a+x $@

$(OUTPUT)/mtest_key: $(SSH_PRIVKEY)
	mkdir -p $(OUTPUT)
	cp $< $@

$(OUTPUT)/bird-node1.conf: bird.conf
	mkdir -p $(OUTPUT)
	sed -e 's/__ASN__/64601/' \
		-e 's/__PEER_ASN__/64602/' \
		-e 's/__PEER_ADDRESS__/$(NODE2)/' \
		$< > $@

$(OUTPUT)/bird-node2.conf: bird.conf
	mkdir -p $(OUTPUT)
	sed -e 's/__ASN__/64602/' \
		-e 's/__PEER_ASN__/64601/' \
		-e 's/__PEER_ADDRESS__/$(NODE1)/' \
		$< > $@

$(OUTPUT)/cluster.yml: cluster.yml
	mkdir -p $(OUTPUT)
	sed -e 's/__BRIDGE_ADDRESS__/$(BRIDGE_ADDRESS)/' \
		-e 's/__COREOS_IMAGE__/$(COREOS_IMAGE)/' $< > $@

$(OUTPUT)/host1.ign: host.ign
	mkdir -p $(OUTPUT)
	sed -e 's/__BRIDGE_ADDRESS__/$(BRIDGE_ADDRESS)/' \
		-e 's/__NODE_NAME__/host1/' \
		-e 's/__NODE1__/$(NODE1)/' \
		-e 's/__NODE2__/$(NODE2)/' \
		-e 's/__NODE_ADDRESS__/$(HOST1)/' $< > $@

$(CT):
	curl -fsSL -o ct https://github.com/coreos/container-linux-config-transpiler/releases/download/v$(CT_VERSION)/ct-v$(CT_VERSION)-x86_64-unknown-linux-gnu
	chmod +x ct
	$(SUDO) mv ct $@

$(OUTPUT)/node.ign: node-ign.yml $(CT)
	mkdir -p $(OUTPUT)
	$(CT) -in-file=$< -pretty -out-file=$@

$(OUTPUT)/node1.ign: $(OUTPUT)/node.ign
	mkdir -p $(OUTPUT)
	sed -e 's/__BRIDGE_ADDRESS__/$(BRIDGE_ADDRESS)/' \
		-e 's/__NODE_NAME__/node1/' \
		-e 's/__NODE1__/$(NODE1)/' \
		-e 's/__NODE2__/$(NODE2)/' \
		-e 's/__NODE_ADDRESS__/$(NODE1)/' $< > $@

$(OUTPUT)/node2.ign: $(OUTPUT)/node.ign
	mkdir -p $(OUTPUT)
	sed -e 's/__BRIDGE_ADDRESS__/$(BRIDGE_ADDRESS)/' \
		-e 's/__NODE_NAME__/node2/' \
		-e 's/__NODE1__/$(NODE1)/' \
		-e 's/__NODE2__/$(NODE2)/' \
		-e 's/__NODE_ADDRESS__/$(NODE2)/' $< > $@

$(OUTPUT)/cke-cluster.yml: cke-cluster.yml
	mkdir -p $(OUTPUT)
	sed -e 's/__NODE1__/$(NODE1)/' \
		-e 's/__NODE2__/$(NODE2)/' \
		-e 's/__RUNTIME__/$(CONTAINER_RUNTIME)/' \
		-e 's|__RUNTIME_ENDPOINT__|$(RUNTIME_ENDPOINT)|' $< > $@

$(OUTPUT)/cke.yml: cke.yml
	mkdir -p $(OUTPUT)
	sed -e 's/__HOST1__/$(HOST1)/' $< > $@

$(OUTPUT)/deploy.yml: deploy.yml
	mkdir -p $(OUTPUT)
	cp $< $@

$(PROTOC):
	curl -fsSL -o /tmp/protobuf.tar.gz https://github.com/protocolbuffers/protobuf/archive/v$(PROTOBUF_VERSION).tar.gz
	tar -x -z -f /tmp/protobuf.tar.gz -C /tmp
	(cd /tmp/protobuf-$(PROTOBUF_VERSION); ./autogen.sh && ./configure && make && $(SUDO) make install)
	$(SUDO) ldconfig
	rm -rf /tmp/protobuf.tar.gz /tmp/protobuf-$(PROTOBUF_VERSION)

build-containerd: $(PROTOC)
	rm -rf $(GOPATH)/src/github.com/containerd/containerd
	mkdir -p $(GOPATH)/src/github.com/containerd/containerd
	curl -fsSL -o containerd.tar.gz https://github.com/containerd/containerd/archive/v${CONTAINERD_VERSION}.tar.gz
	tar -x -z --strip-components 1 -f containerd.tar.gz -C $(GOPATH)/src/github.com/containerd/containerd
	rm -f containerd.tar.gz
	(cd $(GOPATH)/src/github.com/containerd/containerd; env GO111MODULE=off make VERSION=$(CONTAINERD_VERSION))

$(OUTPUT)/containerd: build-containerd
	mkdir -p $(OUTPUT)
	cp $(GOPATH)/src/github.com/containerd/containerd/bin/containerd $@

$(OUTPUT)/containerd-shim: build-containerd
	mkdir -p $(OUTPUT)
	cp $(GOPATH)/src/github.com/containerd/containerd/bin/containerd-shim $@

$(OUTPUT)/ctr: build-containerd
	mkdir -p $(OUTPUT)
	cp $(GOPATH)/src/github.com/containerd/containerd/bin/ctr $@

$(OUTPUT)/crictl:
	mkdir -p $(OUTPUT)
	curl -fsSL -o crictl.tar.gz https://github.com/kubernetes-sigs/cri-tools/releases/download/v$(CRITOOLS_VERSION)/crictl-v$(CRITOOLS_VERSION)-linux-amd64.tar.gz
	tar zxvf crictl.tar.gz -C $(OUTPUT)

# Targets
test: $(TEST_DEPS)
	./test.sh "$(TARGET)"

placemat: $(PLACEMAT_DEPS)
	sudo rm -rf $(PLACEMAT_DATADIR)
	sudo rm -rf $(VM_SHARED_DIR)
	sudo systemd-run --unit=placemat.service $(PLACEMAT) -enable-virtfs $(abspath output/cluster.yml)
	@echo 'Placemat is running.  To kill it, do "make stop".'

stop:
	sudo systemctl stop placemat.service
	sudo systemctl disable placemat.service
	sudo rm /run/systemd/transient/placemat.service
	sudo systemctl daemon-reload
	sudo systemctl reset-failed placemat.service

clean:
	rm -rf $(OUTPUT)

setup:
	go install github.com/onsi/ginkgo/ginkgo
	$(SUDO) apt-get update
	$(SUDO) apt-get -y install --no-install-recommends $(PACKAGES)

.PHONY: all test placemat stop clean setup
