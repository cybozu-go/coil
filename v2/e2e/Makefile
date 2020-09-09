IMAGE = kindest/node:v1.18.8

KIND := $(PWD)/../bin/kind
KUBECTL := $(PWD)/../bin/kubectl
KUSTOMIZE := $(PWD)/../bin/kustomize
export KIND KUBECTL

start:
	sed 's,@IMAGE@,$(IMAGE),g' kind-config-template.yaml > kind-config.yaml
	$(KIND) create cluster --name coil --config kind-config.yaml

stop:
	$(KIND) delete cluster --name coil

install-coil:
	$(KIND) load docker-image --name coil coil:dev
	$(KUSTOMIZE) --load_restrictor=none build . | $(KUBECTL) apply -f -

.PHONY: start stop
