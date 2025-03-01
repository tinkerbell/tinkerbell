#!/bin/bash

set -eou pipefail

function usage() {
        echo "Usage: $0 [OPTION]..."
        echo "Script for creating kube-apiserver certificates and a Kubeconfig"
        echo
        echo "Options:"
        echo "  -i, --apiserver           The Name or IP address of the kube-apiserver (default: localhost)"
        echo "  -d, --dir                 Directory where all files will be created (default: ./certs)"
        echo "  -h, --help                Display this help and exit"
    }

# default values
certs_dir=$(pwd)/certs
apiserver_ip="localhost"

args=$(getopt -a -o i:d:h --long i:,dir:,help -- "$@")
if [[ $? -gt 0 ]]; then
  usage
fi

eval set -- ${args}
while :
do
  case $1 in
    -i | --apiserver)
      if [[ ! -z $2 ]]; then
        apiserver_ip=$2
      fi
      shift 2 ;;
    -d | --dir)
      if [[ ! -z $2 ]]; then
        certs_dir=$2
      fi
      shift 2 ;;
    -h | --help)
      usage
      exit 1
      shift ;;
    # -- means the end of the arguments; drop this, and break out of the while loop
    --) shift; break ;;
    *) >&2 echo Unsupported option: $1
       usage ;;
  esac
done

mkdir -p "$certs_dir"

alt_name="DNS.9 = $apiserver_ip"
# determine if the apiserver_ip is an IP or a hostname
if [[ $apiserver_ip =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "IP address detected"
    alt_name="IP.2 = $apiserver_ip"
fi

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
$alt_name

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

# This creates a kubeconfig that will point to the name or IP address passed in via the -i flag for the apiserver
kubectl config set-cluster local-apiserver --certificate-authority="$certs_dir"/ca.crt --embed-certs=true --server=https://"$apiserver_ip":6443 --kubeconfig="$certs_dir"/kubeconfig
kubectl config set-credentials admin --client-certificate="$certs_dir"/server.crt --client-key="$certs_dir"/server.key --embed-certs=true --kubeconfig="$certs_dir"/kubeconfig
kubectl config set-context default --cluster=local-apiserver --user=admin --kubeconfig="$certs_dir"/kubeconfig
kubectl config use-context default --kubeconfig="$certs_dir"/kubeconfig && chmod 644 "$certs_dir"/kubeconfig

# This creates a kubeconfig that will point to localhost for the apiserver
kubeconfig_localhost="kubeconfig.localhost"
kubectl config set-cluster local-apiserver --certificate-authority="$certs_dir"/ca.crt --embed-certs=true --server=https://localhost:6443 --kubeconfig="$certs_dir/$kubeconfig_localhost"
kubectl config set-credentials admin --client-certificate="$certs_dir"/server.crt --client-key="$certs_dir"/server.key --embed-certs=true --kubeconfig="$certs_dir/$kubeconfig_localhost"
kubectl config set-context default --cluster=local-apiserver --user=admin --kubeconfig="$certs_dir/$kubeconfig_localhost"
kubectl config use-context default --kubeconfig="$certs_dir/$kubeconfig_localhost" && chmod 644 "$certs_dir/$kubeconfig_localhost"
