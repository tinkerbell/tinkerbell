# Embedded Services

Tinkerbell can be built with a few embedded services. This is useful when a single binary with no external dependencies is desired.
The following embedded services are available:

- Kubernetes API server - This is the main API server for Kubernetes. It provides the API for Custom Resource definitions and objects that all other services in Tinkerbell use.
- Kubernetes controller manager - This service provides needed garbage collection and other functionality to the Kubernetes API server.
- ETCD - This is the database that the Kubernetes API server uses to store all of its data.

## Building Tinkerbell with embedded services

To build Tinkerbell with all embedded services, you need to set the `GO_TAGS` Make variable to `embedded`. For example:

```bash
# This builds Tinkerbell with all embedded services using the Host architecture.
# The built binary is located here: out/tinkerbell
make build GO_TAGS=embedded

# This builds Tinkerbell with all embedded services for both amd64 and arm64 architectures.
# The built binaries are located here: out/tinkerbell-linux-amd64, out/tinkerbell-linux-arm64
make cross-compile GO_TAGS=embedded

# Run the Tinkerbell service with all embedded services using the `go run` command.
sudo -E go run -tags=embedded ./cmd/tinkerbell
```

## Disabling embedded services

Once services are embedded, at runtime, you can disable them selectively by setting ENV variables or CLI flags. For example:

```bash
# disable the embedded kube-apiserver and kube-controller-manager via the CLI flag
--enable-embedded-kube-apiserver=false
# disable the embedded kube-apiserver and kube-controller-manager via the ENV variable
export TINKERBELL_ENABLE_EMBEDDED_KUBE_APISERVER=false

# disables the embedded etcd
--enable-embedded-etcd=false
# disable the embedded etcd via the ENV variable
export TINKERBELL_ENABLE_EMBEDDED_ETCD=false
```

## Kubernetes API server certificates

When using the embedded Kubernetes API server, a few ENV vars or CLI flags will need to be set. This includes the etcd end point, a certificate and key for the API server, and a service account key. These can be set as follows:

> [!NOTE]  
> The script `./script/certs.sh` can be used for generating test certificates. Run `./script/certs.sh --help` for options.

```bash
# Environment variables
export TINKERBELL_ETCD_SERVERS=http://localhost:2379
export TINKERBELL_SERVICE_ACCOUNT_KEY_FILE=script/certs/service-account-key.pem
export TINKERBELL_SERVICE_ACCOUNT_SIGNING_KEY_FILE=script/certs/service-account-key.pem
export TINKERBELL_SERVICE_ACCOUNT_ISSUER=api
export TINKERBELL_TLS_CERT_FILE=script/certs/server.crt
export TINKERBELL_TLS_PRIVATE_KEY_FILE=script/certs/server.key
export TINKERBELL_CLIENT_CA_FILE=script/certs/ca.crt
export TINKERBELL_LOGGING_FORMAT=json
export TINKERBELL_BACKEND_KUBE_CONFIG=script/certs/kubeconfig

# CLI flags
--etcd-servers=http://localhost:2379
--service-account-key-file=script/certs/service-account-key.pem
--service-account-signing-key-file=script/certs/service-account-key.pem
--service-account-issuer=api
--tls-cert-file=script/certs/server.crt
--tls-private-key-file=script/certs/server.key
--client-ca-file=script/certs/ca.crt
--logging-format=json
--backend-kube-config=script/certs/kubeconfig
```

To further configure the embedded Kubernetes API server, please refer to the [Kubernetes API server documentation](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-apiserver/) and/or the Tinkerbell help command `tinkerbell --help`.

## ETCD

The embedded etcd server is configured by default to only listen on localhost, no TLS is enabled, and its data directory is `/tmp/default.etcd`. At the moment, only the data directory is configurable. See the Tinkerbell help command `tinkerbell --help` for more information on the embedded etcd server configuration options.

## Example

The following is a working example of running Tinkerbell with all embedded services enabled.

```bash
# Create all needed certs
mkdir certs
wget https://raw.githubusercontent.com/tinkerbell/tinkerbell/refs/heads/main/script/certs.sh -O certs/certs.sh
chmod +x certs/certs.sh
./certs/certs.sh -d ./certs

# Set ENV variables
export TINKERBELL_ETCD_SERVERS=http://localhost:2379
export TINKERBELL_SERVICE_ACCOUNT_KEY_FILE=certs/service-account-key.pem
export TINKERBELL_SERVICE_ACCOUNT_SIGNING_KEY_FILE=certs/service-account-key.pem
export TINKERBELL_SERVICE_ACCOUNT_ISSUER=api
export TINKERBELL_TLS_CERT_FILE=certs/server.crt
export TINKERBELL_TLS_PRIVATE_KEY_FILE=certs/server.key
export TINKERBELL_CLIENT_CA_FILE=certs/ca.crt
export TINKERBELL_LOGGING_FORMAT=json
export TINKERBELL_BACKEND_KUBE_CONFIG=certs/kubeconfig
export TINKERBELL_IPXE_HTTP_SCRIPT_EXTRA_KERNEL_ARGS=tink_worker_image=ghcr.io/tinkerbell/tink-agent:latest
export HOOKOS_NGINX_PORT=32768
export TINKERBELL_IPXE_HTTP_SCRIPT_OSIE_URL=http://"${HOST_IP:-$(hostname -I | awk '{print $1}')}":"${HOOKOS_NGINX_PORT}"

# Download and extract all HookOS artifacts
# This example only downloads the x86_64 artifacts.
mkdir -p hookos
wget https://github.com/tinkerbell/hook/releases/download/latest/hook_latest-lts-x86_64.tar.gz -O hookos/hook.tar.gz
tar -xvf hookos/hook.tar.gz -C hookos
rm hookos/hook.tar.gz
(cd hookos; ln -nfs ./initramfs-latest-lts-x86_64 initramfs-x86_64)
(cd hookos; ln -nfs ./vmlinuz-latest-lts-x86_64 vmlinuz-x86_64)
docker run -d --name image-server -p "${HOOKOS_NGINX_PORT}":80 -v "${PWD}"/hookos:/usr/share/nginx/html/ nginx

# Run Tinkerbell
# sudo is needed as ports for things like TFTP and DHCP are by default privileged ports.
sudo -E ./tinkerbell-embedded-linux-amd64

# In a different terminal, set your KUBECONFIG to use the embedded kube-apiserver
export KUBECONFIG=certs/kubeconfig
```
