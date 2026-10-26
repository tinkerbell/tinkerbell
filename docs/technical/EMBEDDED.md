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

The embedded Kubernetes API server needs a CA, a serving certificate and key, and a service account signing key, and Tinkerbell needs a kubeconfig to reach it.
On first start, Tinkerbell generates all of them in the directory set by `--kube-apiserver-pki-dir` (`TINKERBELL_KUBE_APISERVER_PKI_DIR`), which defaults to `./tinkerbell-pki`, and configures the embedded services to use them.
Later starts reuse the existing files. The embedded Kubernetes API server still needs the etcd endpoint, and its log format can be set to match Tinkerbell's:

```bash
# Environment variables
export TINKERBELL_ETCD_SERVERS=http://localhost:2379
export TINKERBELL_LOGGING_FORMAT=json

# CLI flags
--etcd-servers=http://localhost:2379
--logging-format=json
```

The directory holds the following files:

| File | Purpose |
|---|---|
| `ca.crt`, `ca.key` | CA that signs the serving and client certificates. |
| `server.crt`, `server.key` | Kubernetes API server serving certificate and key. |
| `service-account-key.pem` | Service account token signing key. |
| `kubeconfig` | Admin kubeconfig pointing to `https://localhost:<secure-port>`, or to the `--kube-apiserver-bind-address` when that is a specific IP address. |

The serving certificate is valid for `localhost`, the loopback addresses, the in-cluster `kubernetes` Service names, the host name, the `--kube-apiserver-bind-address` when that is a specific IP address, and the public IP addresses (`--public-ip-v4`, `--public-ip-v6`, or the detected ones). The public IPv6 address is only included when `--listener-families` is `ipv6` or `dual`. To reach the Kubernetes API server from another host, copy the kubeconfig and change its `server` to one of those addresses.
Certificates are valid for 10 years.

Existing files are never changed, so new settings, such as a different public IP, only take effect after the directory is removed. Removing it generates a new CA and service account key, which invalidates previously copied kubeconfigs and issued service account tokens.
If only some of the files exist, Tinkerbell refuses to start rather than generate a CA that the existing files do not trust.

Tinkerbell also refuses to start if the directory or any of its files could be changed by another user, because that user could replace the CA or the kubeconfig and take control of the Kubernetes API server:

- The directory must be a directory, not a symbolic link, and its files must be regular files.
- The directory and its files must be owned by the user running Tinkerbell, or by root, and must not be writable by their group or by others.

A directory that Tinkerbell creates meets these requirements. A directory you create yourself may not: with a umask such as `002`, `mkdir` makes it group-writable. Fix it with `chmod go-w`. When running Tinkerbell with `sudo`, a directory created by a previous run as your own user is not owned by root, so either remove it or change its owner with `sudo chown -R root:root`.

> [!WARNING]
> The kubeconfig authenticates with a client certificate in the `system:masters` group, which bypasses authorization.
> Keep the directory private.

To use your own certificates instead, set the following flags. If any of `--kube-apiserver-tls-cert-file`, `--kube-apiserver-tls-private-key-file`, `--client-ca-file`, `--service-account-key-file`, `--service-account-signing-key-file`, or `--backend-kube-config` is set to a non-empty value, nothing is generated and all of them must be provided. An environment variable that is set but empty, for example `TINKERBELL_BACKEND_KUBE_CONFIG=""`, counts as not set.

> [!NOTE]  
> The script `./script/certs.sh` can be used for generating certificates. Run `./script/certs.sh --help` for options.

```bash
# Environment variables
export TINKERBELL_SERVICE_ACCOUNT_KEY_FILE=script/certs/service-account-key.pem
export TINKERBELL_SERVICE_ACCOUNT_SIGNING_KEY_FILE=script/certs/service-account-key.pem
export TINKERBELL_SERVICE_ACCOUNT_ISSUER=api
export TINKERBELL_KUBE_APISERVER_TLS_CERT_FILE=script/certs/server.crt
export TINKERBELL_KUBE_APISERVER_TLS_PRIVATE_KEY_FILE=script/certs/server.key
export TINKERBELL_CLIENT_CA_FILE=script/certs/ca.crt
export TINKERBELL_BACKEND_KUBE_CONFIG=script/certs/kubeconfig

# CLI flags
--service-account-key-file=script/certs/service-account-key.pem
--service-account-signing-key-file=script/certs/service-account-key.pem
--service-account-issuer=api
--kube-apiserver-tls-cert-file=script/certs/server.crt
--kube-apiserver-tls-private-key-file=script/certs/server.key
--client-ca-file=script/certs/ca.crt
--backend-kube-config=script/certs/kubeconfig
```

To further configure the embedded Kubernetes API server, please refer to the [Kubernetes API server documentation](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-apiserver/) and/or the Tinkerbell help command `tinkerbell --help`.

## ETCD

The embedded etcd server is configured by default to only listen on localhost, no TLS is enabled, and its data directory is `/tmp/default.etcd`. At the moment, only the data directory is configurable. See the Tinkerbell help command `tinkerbell --help` for more information on the embedded etcd server configuration options.

## Example

The following is a working example of running Tinkerbell with all embedded services enabled.

```bash
# Set ENV variables
# The Kubernetes API server certificates are generated in ./tinkerbell-pki on first start.
export TINKERBELL_ETCD_SERVERS=http://localhost:2379
export TINKERBELL_LOGGING_FORMAT=json
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

# In a different terminal, use the kubeconfig generated on first start.
# It is only readable by its owner, root when Tinkerbell is run with sudo.
sudo cp tinkerbell-pki/kubeconfig ./kubeconfig && sudo chown "$(id -u):$(id -g)" ./kubeconfig
export KUBECONFIG=./kubeconfig
```
