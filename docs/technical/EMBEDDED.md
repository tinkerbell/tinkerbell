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
