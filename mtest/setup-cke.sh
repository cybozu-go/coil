#!/bin/sh -e

VAULT=/data/vault
CKECLI=/opt/bin/ckecli

if [ ! -f /usr/bin/jq ]; then
    echo "please wait; cloud-init will install jq."
    exit 1
fi

run_etcd() {
    sudo systemctl is-active my-etcd.service && return 0
    sudo systemd-run --unit=my-etcd.service /data/etcd --data-dir /home/cybozu/default.etcd
}

create_ca() {
    ca="$1"
    common_name="$2"
    key="$3"

    $VAULT secrets enable -path $ca -max-lease-ttl=876000h -default-lease-ttl=87600h pki
    $VAULT write -format=json "$ca/root/generate/internal" \
           common_name="$common_name" \
           ttl=876000h format=pem | jq -r .data.certificate > /tmp/ca.pem
    $CKECLI ca set $key /tmp/ca.pem
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
    cat > /home/cybozu/cke-policy.hcl <<'EOF'
path "cke/*"
{
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}
EOF
    $VAULT policy write cke /home/cybozu/cke-policy.hcl
    $VAULT write auth/approle/role/cke policies=cke period=5s
    role_id=$($VAULT read -format=json auth/approle/role/cke/role-id | jq -r .data.role_id)
    secret_id=$($VAULT write -f -format=json auth/approle/role/cke/secret-id | jq -r .data.secret_id)
    $CKECLI vault config - <<EOF
{
    "endpoint": "http://127.0.0.1:8200",
    "role-id": "$role_id",
    "secret-id": "$secret_id"
}
EOF

    create_ca cke/ca-server "server CA" server
    create_ca cke/ca-etcd-peer "etcd peer CA" etcd-peer
    create_ca cke/ca-etcd-client "etcd client CA" etcd-client
    create_ca cke/ca-kubernetes "kubernetes CA" kubernetes

    # admin role need to be created here to generate .kube/config
    $VAULT write cke/ca-kubernetes/roles/admin ttl=2h max_ttl=24h \
           enforce_hostnames=false allow_any_name=true organization=system:masters
}

install_cke_configs() {
  sudo tee /etc/cke/config.yml >/dev/null <<EOF
endpoints: ["http://127.0.0.1:2379"]
EOF
}

install_kubectl_config() {
    mkdir -p $HOME/.kube
    $CKECLI kubernetes issue >$HOME/.kube/config
}

install_ckecli() {
    docker run --rm -u root:root --entrypoint /usr/local/cke/install-tools \
           -v /opt/bin:/host \
           quay.io/cybozu/cke:0
}

run_cke() {
    docker inspect cke >/dev/null && return 0
    docker run -d --rm --name cke --net=host -v /etc/cke:/etc/cke:ro quay.io/cybozu/cke:0 -interval 2s
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
install_kubectl_config

cat <<EOF

CKE has been initialized. use kubectl to manage a kubernetes cluster as:

    $ /data/kubectl api-resources

Run setup-coil.sh to setup etcd certificates for Coil.

    $ /data/setup-coil.sh

EOF
