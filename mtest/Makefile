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
K8S_VERSION = 1.13.2
PLACEMAT_DATADIR = /var/scratch/placemat
CONTAINERD_VERSION = 1.2.4
PROTOC_VERSION = 3.6.1
CRITOOLS_VERSION = 1.13.0
CT_VERSION = 0.9.0
SUDO = sudo
PACKAGES = libseccomp-dev
CT = /usr/local/bin/ct

export BRIDGE_ADDRESS HOST1 NODE1 NODE2 PLACEMAT GINKGO CONTAINER_RUNTIME

# non-configuration variables
MANAGEMENT_ETCD_ARCHIVE := etcd-v$(MANAGEMENT_ETCD_VERSION)-linux-amd64.tar.gz
VAULT_ARCHIVE := vault_$(VAULT_VERSION)_linux_amd64.zip
KUBECTL_URL := https://storage.googleapis.com/kubernetes-release/release/v$(K8S_VERSION)/bin/linux/amd64/kubectl
SSH_PRIVKEY := ./mtest_key
OUTPUT := ./output
COREOS_IMAGE := coreos_production_qemu_image.img
CKECLI := $(OUTPUT)/ckecli
CKECONFIG := $(OUTPUT)/cke.yml
KUBECTL := $(OUTPUT)/kubectl
COILIMAGE := $(OUTPUT)/coil.img

export SSH_PRIVKEY CKECLI CKECONFIG COILIMAGE KUBECTL

GENERATED_FILES = \
	$(COILIMAGE) $(OUTPUT)/coilctl \
	$(OUTPUT)/etcd  $(OUTPUT)/etcdctl \
	$(OUTPUT)/vault \
	$(OUTPUT)/kubectl \
	$(OUTPUT)/cluster.yml \
	$(OUTPUT)/host1.ign \
	$(OUTPUT)/node1.ign \
	$(OUTPUT)/node2.ign \
	$(OUTPUT)/cke-cluster.yml \
	$(OUTPUT)/cke.yml \
	$(OUTPUT)/setup-cke.sh \
	$(OUTPUT)/setup-coil.sh \
	$(OUTPUT)/bird-node1.conf \
	$(OUTPUT)/bird-node2.conf \
	$(OUTPUT)/rbac.yml $(OUTPUT)/deploy.yml \
	$(OUTPUT)/mtest_key \
	$(COREOS_IMAGE) \

all: test

$(OUTPUT)/coil.img:
	rm -rf tmpbin
	mkdir tmpbin
	cd ..; GOBIN=$(shell pwd)/tmpbin CGO_ENABLED=0 go install -mod=vendor ./cmd/hypercoil
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

node.ign: node-ign.yml
	$(CT) -in-file=$< -pretty -out-file=$@

$(OUTPUT)/node1.ign: node.ign
	mkdir -p $(OUTPUT)
	sed -e 's/__BRIDGE_ADDRESS__/$(BRIDGE_ADDRESS)/' \
		-e 's/__NODE_NAME__/node1/' \
		-e 's/__NODE1__/$(NODE1)/' \
		-e 's/__NODE2__/$(NODE2)/' \
		-e 's/__NODE_ADDRESS__/$(NODE1)/' $< > $@

$(OUTPUT)/node2.ign: node.ign
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

$(OUTPUT)/rbac.yml: rbac.yml
	mkdir -p $(OUTPUT)
	cp $< $@

$(OUTPUT)/deploy.yml: deploy.yml
	mkdir -p $(OUTPUT)
	cp $< $@

test: $(GENERATED_FILES)
	sudo rm -rf $(PLACEMAT_DATADIR)/volumes/host1
	sudo rm -rf $(PLACEMAT_DATADIR)/volumes/node1
	sudo rm -rf $(PLACEMAT_DATADIR)/volumes/node2
	./test.sh "$(TARGET)"

placemat: $(GENERATED_FILES)
	sudo rm -rf $(PLACEMAT_DATADIR)/volumes/host1
	sudo rm -rf $(PLACEMAT_DATADIR)/volumes/node1
	sudo rm -rf $(PLACEMAT_DATADIR)/volumes/node2
	sudo -b $(PLACEMAT) -force output/cluster.yml
	@echo 'Placemat is running.  To kill it, do "sudo pkill placemat".'

test-light:
	$(GINKGO) -v -focus="${TARGET}"

clean:
	rm -rf $(OUTPUT) node.ign

setup:
	GO111MODULE=off go get -u github.com/onsi/ginkgo/ginkgo
	$(SUDO) apt-get update
	$(SUDO) apt-get -y install --no-install-recommends $(PACKAGES)
	curl -fsSL -o protoc.zip https://github.com/google/protobuf/releases/download/v$(PROTOC_VERSION)/protoc-$(PROTOC_VERSION)-linux-x86_64.zip
	$(SUDO) unzip -o protoc.zip -d /usr/local
	curl -fsSL -o ct https://github.com/coreos/container-linux-config-transpiler/releases/download/v$(CT_VERSION)/ct-v$(CT_VERSION)-x86_64-unknown-linux-gnu
	chmod +x ct
	$(SUDO) mv ct /usr/local/bin

.PHONY:	all clean setup test
