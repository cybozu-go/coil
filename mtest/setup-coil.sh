#!/bin/sh -e

CKECLI=/opt/bin/ckecli
KUBECTL=/data/kubectl
COILCTL=/data/coilctl

checkKubernetes() {
    if $KUBECTL get nodes >/dev/null 2>&1; then
        return
    fi
    echo "Kubernetes is not ready"
    exit 2
}

setupCerts() {
    $CKECLI etcd user-add coil /coil/
    certs=$($CKECLI etcd issue coil)

    cat >$HOME/.coilctl.yml <<EOF
endpoints:
  - https://10.0.0.101:2379
tls-ca: $(echo "$certs" | jq .ca_certificate)
tls-cert: $(echo "$certs" | jq .certificate)
tls-key: $(echo "$certs" | jq .private_key)
EOF

    $KUBECTL create -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: coil-etcd-secrets
type: Opaque
data:
  cacert: $(echo "$certs" | jq -r .ca_certificate | base64 -w 0)
  cert: $(echo "$certs" | jq -r .certificate | base64 -w 0)
  key: $(echo "$certs" | jq -r .private_key | base64 -w 0)
EOF
}

# main
checkKubernetes
setupCerts
