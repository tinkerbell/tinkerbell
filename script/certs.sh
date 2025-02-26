#!/bin/bash

set -eou pipefail

mkdir -p certs

certs_dir=$(pwd)/certs

cat > "$certs_dir"/csr.conf <<EOF
[ req ]
default_bits = 2048
prompt = no
default_md = sha256
req_extensions = req_ext
distinguished_name = dn

[ dn ]
C = US
ST = CA
L = Los Angeles
O = Tinkerbell
OU = Engineering
CN = localhost

[ req_ext ]
subjectAltName = @alt_names

[ alt_names ]
DNS.1 = kubernetes
DNS.2 = kubernetes.default
DNS.3 = kubernetes.default.svc
DNS.4 = kubernetes.default.svc.cluster
DNS.5 = kubernetes.default.svc.cluster.local
DNS.6 = kube-apiserver
DNS.7 = tinkerbell
DNS.8 = localhost
IP.1 = 127.0.0.1
IP.2 = 192.168.2.50

[ v3_ext ]
authorityKeyIdentifier=keyid,issuer:always
basicConstraints=CA:FALSE
keyUsage=keyEncipherment,dataEncipherment
extendedKeyUsage=serverAuth,clientAuth
subjectAltName=@alt_names
EOF

openssl genrsa -out "$certs_dir"/service-account-key.pem 4096
openssl req -new -x509 -days 365 -key "$certs_dir"/service-account-key.pem -subj "/CN=test" -sha256 -out "$certs_dir"/service-account.pem
openssl genrsa -out "$certs_dir"/ca.key 2048
openssl req -x509 -new -nodes -key "$certs_dir"/ca.key -subj "/CN=test" -days 10000 -out "$certs_dir"/ca.crt
openssl genrsa -out "$certs_dir"/server.key 2048
openssl req -new -key "$certs_dir"/server.key -out "$certs_dir"/server.csr -config "$certs_dir"/csr.conf
openssl x509 -req -in "$certs_dir"/server.csr -CA "$certs_dir"/ca.crt -CAkey "$certs_dir"/ca.key -CAcreateserial -out "$certs_dir"/server.crt -days 10000 -extensions v3_ext -extfile "$certs_dir"/csr.conf

kubectl config set-cluster local-apiserver --certificate-authority=certs/ca.crt --embed-certs=true --server=https://localhost:6443 --kubeconfig=kubeconfig
kubectl config set-credentials admin --client-certificate=certs/server.crt --client-key=certs/server.key --embed-certs=true --kubeconfig=kubeconfig
kubectl config set-context default --cluster=local-apiserver --user=admin --kubeconfig=kubeconfig
kubectl config use-context default --kubeconfig=kubeconfig && chmod 644 kubeconfig
