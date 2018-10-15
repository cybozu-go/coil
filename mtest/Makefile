# Makefile for multi-host testing using ginkgo

# configuration variables
BRIDGE_ADDRESS = 10.0.0.1
## 'HOST' runs CKE and management etcd
HOST1 = 10.0.0.11
## 'NODE' runs etcd and k8s, managed by CKE
NODE1 = 10.0.0.101
NODE2 = 10.0.0.102
PLACEMAT = $(GOPATH)/bin/placemat
GINKGO = env GOFLAGS=-mod=vendor $(GOPATH)/bin/ginkgo
MANAGEMENT_ETCD_VERSION = 3.3.9
VAULT_VERSION = 0.11.0
K8S_VERSION = 1.11.2
PLACEMAT_DATADIR = /var/scratch/placemat

export BRIDGE_ADDRESS HOST1 NODE1 NODE2 PLACEMAT GINKGO

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
	$(COREOS_IMAGE) \

all: test

$(OUTPUT)/coil.img:
	rm -rf tmpbin
	mkdir tmpbin
	cd ..; GOBIN=$(shell pwd)/tmpbin CGO_ENABLED=0 go install -mod=vendor ./...
	sudo podman build --rm=false -f Dockerfile -t quay.io/cybozu/coil:latest tmpbin
	mkdir -p $(OUTPUT)
	sudo podman save -o $@ quay.io/cybozu/coil:latest

$(OUTPUT)/coilctl: $(OUTPUT)/coil.img
	cp tmpbin/coilctl $(OUTPUT)

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

$(OUTPUT)/bird-node1.conf: bird.conf
	mkdir -p $(OUTPUT)
	sed -e 's/@ASN@/64601/' \
		-e 's/@PEER_ASN@/64602/' \
		-e 's/@PEER_ADDRESS@/$(NODE2)/' \
		$< > $@

$(OUTPUT)/bird-node2.conf: bird.conf
	mkdir -p $(OUTPUT)
	sed -e 's/@ASN@/64602/' \
		-e 's/@PEER_ASN@/64601/' \
		-e 's/@PEER_ADDRESS@/$(NODE1)/' \
		$< > $@

$(OUTPUT)/cluster.yml: cluster.yml
	mkdir -p $(OUTPUT)
	sed -e 's/@BRIDGE_ADDRESS@/$(BRIDGE_ADDRESS)/' \
		-e 's/@COREOS_IMAGE@/$(COREOS_IMAGE)/' $< > $@

$(OUTPUT)/host1.ign: host.ign
	mkdir -p $(OUTPUT)
	sed -e 's/@BRIDGE_ADDRESS@/$(BRIDGE_ADDRESS)/' \
		-e 's/@NODE_NAME@/host1/' \
		-e 's/@NODE1@/$(NODE1)/' \
		-e 's/@NODE2@/$(NODE2)/' \
		-e 's/@NODE_ADDRESS@/$(HOST1)/' $< > $@

$(OUTPUT)/node1.ign: node.ign
	mkdir -p $(OUTPUT)
	sed -e 's/@BRIDGE_ADDRESS@/$(BRIDGE_ADDRESS)/' \
		-e 's/@NODE_NAME@/node1/' \
		-e 's/@NODE1@/$(NODE1)/' \
		-e 's/@NODE2@/$(NODE2)/' \
		-e 's/@NODE_ADDRESS@/$(NODE1)/' $< > $@

$(OUTPUT)/node2.ign: node.ign
	mkdir -p $(OUTPUT)
	sed -e 's/@BRIDGE_ADDRESS@/$(BRIDGE_ADDRESS)/' \
		-e 's/@NODE_NAME@/node2/' \
		-e 's/@NODE1@/$(NODE1)/' \
		-e 's/@NODE2@/$(NODE2)/' \
		-e 's/@NODE_ADDRESS@/$(NODE2)/' $< > $@

$(OUTPUT)/cke-cluster.yml: cke-cluster.yml
	mkdir -p $(OUTPUT)
	sed -e 's/@NODE1@/$(NODE1)/' \
		-e 's/@NODE2@/$(NODE2)/' $< > $@

$(OUTPUT)/cke.yml: cke.yml
	mkdir -p $(OUTPUT)
	sed -e 's/@HOST1@/$(HOST1)/' $< > $@

test: $(GENERATED_FILES)
	sudo rm -rf $(PLACEMAT_DATADIR)/volumes/host1
	sudo rm -rf $(PLACEMAT_DATADIR)/volumes/node1
	sudo rm -rf $(PLACEMAT_DATADIR)/volumes/node2
	./test.sh

placemat: $(GENERATED_FILES)
	sudo rm -rf $(PLACEMAT_DATADIR)/volumes/host1
	sudo rm -rf $(PLACEMAT_DATADIR)/volumes/node1
	sudo rm -rf $(PLACEMAT_DATADIR)/volumes/node2
	sudo -b $(PLACEMAT) -force output/cluster.yml
	@echo 'Placemat is running.  To kill it, do "sudo pkill placemat".'

test-light:
	$(GINKGO) -v -focus="${TARGET}"

clean:
	rm -rf $(OUTPUT)

setup:
	GO111MODULE=off go get -u \
		github.com/onsi/ginkgo/ginkgo \
		github.com/cybozu-go/placemat/...

.PHONY:	all clean setup test
