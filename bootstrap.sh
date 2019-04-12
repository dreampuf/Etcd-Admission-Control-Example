#!/usr/bin/env bash

set -euo pipefail

WEBHOOK_SVC='etcd-admission-control.default.svc'
SECRET_NAME='etcd-admission-key'

if [ ! -f "ca.crt" ] || [ ! -f "webhook-server-tls.crt" ] || [ ! -f "webhook-server-tls.key" ]; then
    # Generate the CA cert and private key
    openssl req -nodes -new -x509 -keyout ca.key -out ca.crt -subj "/CN=ETCD Admission CONTROL CA"
    # Generate the private key for the webhook server
    openssl genrsa -out webhook-server-tls.key 2048
    # Generate a Certificate Signing Request (CSR) for the private key, and sign it with the private key of the CA.
    openssl req -new -key webhook-server-tls.key -subj "/CN=${WEBHOOK_SVC}" \
        | openssl x509 -req -CA ca.crt -CAkey ca.key -CAcreateserial -out webhook-server-tls.crt
fi

kubectl get secret ${SECRET_NAME} &> /dev/null || kubectl create secret tls ${SECRET_NAME} \
    --cert "webhook-server-tls.crt" \
    --key "webhook-server-tls.key"

ca_pem_b64="$(openssl base64 -A <"ca.crt")"
sed -e 's@${CA_PEM_B64}@'"$ca_pem_b64"'@g' <"deployment.yaml" \
    | kubectl create -f -

echo "Etcd Admission Control has been deployed."

