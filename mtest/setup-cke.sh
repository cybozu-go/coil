#!/bin/bash -e

VAULT=/data/vault
CKECLI=/opt/bin/ckecli
CKE_VERSION=1.13

if [ ! -f /usr/bin/jq ]; then
    echo "please wait; cloud-init will install jq."
    exit 1
fi

run_etcd() {
    sudo systemctl is-active my-etcd.service && return 0
    sudo systemd-run --unit=my-etcd.service /data/etcd --data-dir /home/cybozu/default.etcd
}

run_vault() {
    sudo systemctl is-active my-vault.service && return 0

    sudo systemd-run --unit=my-vault.service /data/vault server -dev -dev-root-token-id=cybozu

    VAULT_TOKEN=cybozu
    export VAULT_TOKEN
    VAULT_ADDR=http://127.0.0.1:8200
    export VAULT_ADDR

    for i in $(seq 10); do
        sleep 1
        if $VAULT status >/dev/null 2>&1; then
            break
        fi
    done

    $VAULT auth enable approle
    $CKECLI vault init
    $CKECLI vault ssh-privkey /data/mtest_key

    # admin role need to be created here to generate .kube/config
    $VAULT write cke/ca-kubernetes/roles/admin ttl=2h max_ttl=24h \
           enforce_hostnames=false allow_any_name=true organization=system:masters
}

install_cke_configs() {
  sudo tee /etc/cke/config.yml >/dev/null <<EOF
endpoints: ["http://127.0.0.1:2379"]
EOF
}

install_kubectl() {
    sudo cp /data/kubectl /opt/bin/kubectl
    mkdir -p $HOME/.kube
    $CKECLI kubernetes issue >$HOME/.kube/config
}

install_etcdctl() {
    sudo cp /data/etcdctl /opt/bin/etcdctl
}

install_ckecli() {
    docker run --rm -u root:root --entrypoint /usr/local/cke/install-tools \
           -v /opt/bin:/host \
           quay.io/cybozu/cke:${CKE_VERSION}
}

run_cke() {
    set +e
    sudo systemctl reset-failed cke.service
    sudo systemctl stop cke.service
    set -e
    sudo systemd-run --unit=cke.service \
        docker run --rm --name cke --net=host -v /etc/cke:/etc/cke:ro quay.io/cybozu/cke:${CKE_VERSION} --interval 2s
}

setup_cke() {
    $CKECLI constraints set control-plane-count 1
    $CKECLI cluster set /data/cluster.yml
}

install_cke_configs
run_etcd
sleep 1
install_ckecli
run_vault
run_cke
sleep 1
setup_cke
install_kubectl
install_etcdctl
