# TLS Termination

This document describes the TLS termination support in Tinkerbell, including which endpoints and services support TLS, and how to configure them.

## Overview

Tinkerbell supports TLS termination for HTTP and gRPC services. This allows secure communications between clients and Tinkerbell services. When TLS is enabled, both HTTP and HTTPS servers are run concurrently on different ports, providing backward compatibility while adding secure connection options.

## Supported Services

The following services support TLS termination:

1. **Smee HTTP/HTTPS Server**
   - Serves iPXE binaries, scripts, and ISO files over both HTTP and HTTPS
   - Default ports: HTTP (7171), HTTPS (7272)
   
2. **Tink gRPC Server**
   - Secure gRPC API communications
   - Default port: 42113

## TLS Certificate Requirements

When enabling TLS for Smee's iPXE services, note that iPXE only supports RSA certificates. If you provide an ECDSA certificate, a warning will be logged, and iPXE clients may be unable to connect via HTTPS.

## Configuration

### CLI Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--tls-cert-file` | Path to the TLS certificate file | "" |
| `--tls-key-file` | Path to the TLS key file | "" |
| `--https-bind-port` | Port for HTTPS server | 7272 |
| `--dhcp-ipxe-http-script-scheme` | Protocol scheme for iPXE scripts (http or https) | "http" |
| `--ipxe-script-tink-server-use-tls` | Use TLS to connect to the Tink server | false |
| `--ipxe-script-tink-server-insecure-tls` | Skip TLS verification when connecting to the Tink server | false |

### Environment Variables

| Environment Variable | Description | CLI Equivalent |
|---------------------|-------------|----------------|
| `TINKERBELL_TLS_CERT_FILE` | Path to the TLS certificate file | `--tls-cert-file` |
| `TINKERBELL_TLS_KEY_FILE` | Path to the TLS key file | `--tls-key-file` |
| `TINKERBELL_HTTPS_BIND_PORT` | Port for HTTPS server | `--https-bind-port` |
| `TINKERBELL_DHCP_IPXE_HTTP_SCRIPT_SCHEME` | Protocol scheme for iPXE scripts | `--dhcp-ipxe-http-script-scheme` |
| `TINKERBELL_IPXE_SCRIPT_TINK_SERVER_USE_TLS` | Use TLS to connect to Tink server | `--ipxe-script-tink-server-use-tls` |
| `TINKERBELL_IPXE_SCRIPT_TINK_SERVER_INSECURE_TLS` | Skip TLS verification | `--ipxe-script-tink-server-insecure-tls` |

### Helm Chart Values

To configure TLS in the Helm chart, set the following values:

```yaml
deployment:
  envs:
    globals:
      tlsCertFile: "/path/to/cert.crt"
      tlsKeyFile: "/path/to/cert.key"
    smee:
      httpsBindPort: 7272
      dhcpIpxeHttpScriptScheme: "https"  # Use HTTPS for iPXE scripts
      ipxeScriptTinkServerUseTLS: true
      ipxeScriptTinkServerInsecureTLS: false
```

You can also mount certificate files using Kubernetes secrets:

```yaml
deployment:
  volumes:
    - name: tinkerbell-tls
      secret:
        secretName: tinkerbell-tls
  volumeMounts:
    - name: tinkerbell-tls
      mountPath: /tmp/certs
      readOnly: true
  envs:
    globals:
      tlsCertFile: "/tmp/certs/tls.crt"
      tlsKeyFile: "/tmp/certs/tls.key"
```

Using Helm CLI to install Tinkerbell with TLS:

```bash
# Create a TLS secret first
kubectl create secret tls tinkerbell-tls --cert=cert.crt --key=cert.key -n tinkerbell

# Install Tinkerbell with TLS configuration
helm upgrade --install tinkerbell helm/tinkerbell \
  --namespace tinkerbell --create-namespace \
  --set-json 'deployment.volumes=[{"name":"tinkerbell-tls","secret":{"secretName":"tinkerbell-tls"}}]' \
  --set-json 'deployment.volumeMounts=[{"name":"tinkerbell-tls","mountPath":"/tmp/certs","readOnly":true}]' \
  --set "deployment.envs.globals.tlsCertFile=/tmp/certs/tls.crt" \
  --set "deployment.envs.globals.tlsKeyFile=/tmp/certs/tls.key" \
  --set "deployment.envs.smee.dhcpIpxeHttpScriptScheme=https" \
  --set "deployment.envs.smee.ipxeScriptTinkServerUseTLS=true"
```

## HTTPS Endpoints

When TLS is enabled, the following endpoints are available over HTTPS (in addition to HTTP):

| Endpoint | Description |
|----------|-------------|
| `/ipxe/binary/` | Serves iPXE binaries |
| `/ipxe/script/` | Serves iPXE scripts |
| `/iso/` | Serves ISO files |
| `/healthcheck` | Server health information |
| `/metrics` | Prometheus metrics |

## Internal Architecture

### HTTP/HTTPS Server Configuration

Tinkerbell implements dual HTTP/HTTPS servers when TLS is enabled:

1. The HTTP server continues to serve on the default port (7171)
2. An HTTPS server is started on the HTTPS port (7272 by default)
3. Both servers share the same handlers and routes

The TLS configuration uses TLS 1.2 as the minimum version to ensure security while maintaining compatibility with older clients.

### gRPC Server TLS Configuration

The gRPC server uses the provided TLS certificate for securing communications between Tink Agents and the Tink Server. This enables encrypted communication for all gRPC API calls.

## Using TLS with iPXE

When using iPXE with TLS, consider:

1. iPXE only supports RSA certificates (not ECDSA)
2. Set `ipxeScriptTinkServerUseTLS: true` if your Tink server uses TLS
3. For development environments with self-signed certificates, you may need to set `ipxeScriptTinkServerInsecureTLS: true`
4. When configuring DHCP to use HTTPS for iPXE scripts, set `dhcpIpxeHttpScriptScheme: "https"` in Helm values or `--dhcp-ipxe-http-script-scheme=https` via CLI
