# Template Secrets

This document explains how to use Kubernetes Secrets in Tinkerbell Templates to securely inject sensitive data like passwords, API tokens, and other credentials into your workflows.

## Overview

Template Secrets allow you to reference a Kubernetes Secret from a Template resource. All keys from the referenced Secret become available during template rendering, enabling secure access to sensitive data without hardcoding credentials in your templates.

## Why Use Template Secrets?

- **Simplicity**: Direct, straightforward way to inject credentials into templates without complex policy configuration
- **Namespace Isolation**: Templates can only access secrets in the same namespace, providing natural security boundaries

## How to Use Template Secrets

### 1. Create a Kubernetes Secret

First, create a Secret containing the sensitive data you need in your workflow:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: database-credentials
  namespace: default
type: Opaque
stringData:
  db_password: "mySecurePassword123"
  db_username: "admin"
  api_token: "super-secret-token"
  encryption_key: "my-encryption-key-12345"
```

Apply the secret:

```bash
kubectl apply -f secret.yaml
```

### 2. Reference the Secret in a Template

Add a `secretRef` field to your Template specification, specifying the secret name. The secret must exist in the same namespace as the Template:

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Template
metadata:
  name: database-setup
  namespace: default
spec:
  secretRef:
    name: database-credentials
  data: |
    version: "0.1"
    name: database-setup-workflow
    global_timeout: 600
    tasks:
      - name: configure-database
        worker: "{{.hardware.spec.metadata.instance.id}}"
        actions:
          - name: install-database
            image: postgres:15
            timeout: 300
            env:
              POSTGRES_PASSWORD: "{{.secret.db_password}}"
              POSTGRES_USER: "{{.secret.db_username}}"
          - name: configure-app
            image: myapp:latest
            timeout: 300
            env:
              DB_PASSWORD: "{{.secret.db_password}}"
              DB_USERNAME: "{{.secret.db_username}}"
              API_TOKEN: "{{.secret.api_token}}"
              ENCRYPTION_KEY: "{{.secret.encryption_key}}"
```

### 3. Access Secret Values in Templates

Secret values are accessible using the `.secret.<key>` syntax in your template:

- `.secret.db_password` - Access the `db_password` key
- `.secret.db_username` - Access the `db_username` key
- `.secret.api_token` - Access the `api_token` key

All keys from the referenced Secret are automatically exposed to the template.

## Complete Example

Here's a full example demonstrating secret usage in a deployment workflow:

**Secret:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: app-secrets
  namespace: tink-system
type: Opaque
stringData:
  ssh_private_key: |
    -----BEGIN OPENSSH PRIVATE KEY-----
    b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABFwAAAAdzc2gtcn
    ...
    -----END OPENSSH PRIVATE KEY-----
  docker_registry_password: "myDockerPassword"
  tls_cert: |
    -----BEGIN CERTIFICATE-----
    MIIDXTCCAkWgAwIBAgIJAKs...
    -----END CERTIFICATE-----
  tls_key: |
    -----BEGIN PRIVATE KEY-----
    MIIEvQIBADANBgkqhkiG9w0B...
    -----END PRIVATE KEY-----
```

**Template:**
```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Template
metadata:
  name: secure-app-deployment
  namespace: tink-system
spec:
  secretRef:
    name: app-secrets
  data: |
    version: "0.1"
    name: secure-deployment
    global_timeout: 1800
    tasks:
      - name: setup-server
        worker: "{{.hardware.spec.metadata.instance.id}}"
        actions:
          - name: configure-ssh
            image: alpine:latest
            timeout: 120
            command:
              - /bin/sh
              - -c
            args:
              - |
                mkdir -p /root/.ssh
                echo "{{.secret.ssh_private_key}}" > /root/.ssh/id_rsa
                chmod 600 /root/.ssh/id_rsa

          - name: pull-docker-image
            image: docker:latest
            timeout: 300
            env:
              DOCKER_PASSWORD: "{{.secret.docker_registry_password}}"
            command:
              - /bin/sh
              - -c
            args:
              - |
                echo "$DOCKER_PASSWORD" | docker login -u myuser --password-stdin
                docker pull myregistry.com/myapp:latest

          - name: install-tls-certificates
            image: alpine:latest
            timeout: 120
            command:
              - /bin/sh
              - -c
            args:
              - |
                echo "{{.secret.tls_cert}}" > /etc/ssl/certs/app.crt
                echo "{{.secret.tls_key}}" > /etc/ssl/private/app.key
                chmod 600 /etc/ssl/private/app.key
```

**Workflow:**
```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Workflow
metadata:
  name: deploy-secure-app
  namespace: tink-system
spec:
  templateRef: secure-app-deployment
  hardwareRef: server-01
```

## Combining Secrets with Other Template Data

Secret values can be used alongside other template data sources like hardware information and references:

```yaml
spec:
  secretRef:
    name: my-secrets
  data: |
    version: "0.1"
    name: combined-example
    tasks:
      - name: configure
        worker: "{{.hardware.spec.metadata.instance.id}}"
        actions:
          - name: setup
            image: alpine:latest
            env:
              # From secret
              PASSWORD: "{{.secret.password}}"
              # From hardware
              HOSTNAME: "{{.hardware.spec.metadata.instance.hostname}}"
              # From hardware map
              CUSTOM_VAR: "{{.custom_value}}"
              # From references
              LVM_CONFIG: "{{.references.lvm.spec.group}}"
```

## Security Considerations

### RBAC Permissions

The Tinkerbell controller requires the following RBAC permissions to read secrets:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: tinkerbell-controller
  namespace: default
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch"]
```

These permissions are automatically included when using the Tinkerbell Helm chart.

### Best Practices

1. **Namespace Isolation**: Keep secrets in the same namespace as your templates for better access control
2. **Least Privilege**: Only grant the Tinkerbell controller access to secrets it needs
3. **Secret Rotation**: Regularly rotate secrets and update Secret resources
4. **Audit Logging**: Enable Kubernetes audit logging to track secret access
5. **Encryption at Rest**: Enable encryption at rest for Kubernetes secrets
6. **Avoid Logging**: Be careful not to log or output secret values in workflow actions

### What Happens if a Secret is Missing?

If the referenced secret doesn't exist or is inaccessible:

1. The workflow will fail to render the template
2. The Workflow status will show `TemplateRenderingFailed`
3. An error message will indicate the secret couldn't be found
4. The workflow will not proceed to execution

Example error condition:

```yaml
status:
  templateRendering: Failed
  conditions:
    - type: TemplateRenderedSuccess
      status: "False"
      reason: Error
      message: "error fetching secret: secret not found: name=missing-secret, namespace=default"
```

## Limitations

- **Namespace-Scoped Only**: Templates can only access secrets in the same namespace. The secret is always fetched from the Template's namespace.
- **Single Secret Reference**: Each template can reference only one secret. If you need values from multiple secrets, combine them into a single secret.
- **String Values Only**: All secret data is converted to strings for template rendering. Binary data should be base64 encoded.

## Template Secrets vs. Hardware References

Both Template Secrets and Hardware References can be used to inject data into templates, but they serve different purposes:

| Feature | Template Secrets | Hardware References |
|---------|-----------------|---------------------|
| **Purpose** | Simple credential injection | Flexible access to any Kubernetes resource |
| **Scope** | Namespace-scoped only | Cross-namespace with policy controls |
| **Access Control** | Namespace isolation | Allowlist/denylist policy rules |
| **Configuration** | Zero configuration needed | Requires allowlist/denylist configuration |
| **Use Case** | Passwords, tokens, certificates | LVM configs, network bonds, any CRD |
| **Complexity** | Simple and straightforward | More complex but more flexible |

**When to use Template Secrets:**
- You need to inject credentials within the same namespace
- You want simple, zero-configuration secret access
- Namespace isolation is sufficient for your security requirements

**When to use Hardware References:**
- You need to access resources across namespaces
- You need fine-grained policy control over resource access
- You're accessing non-Secret resources (ConfigMaps, CRDs, etc.)
- You need cluster-wide resource access with explicit allowlisting

## Alternative: Using Kubernetes ConfigMaps

For non-sensitive configuration data, consider using Hardware References to link to ConfigMaps instead:

```yaml
# Hardware with ConfigMap reference
spec:
  references:
    config:
      group: ""
      name: app-config
      namespace: default
      resource: configmaps
      version: v1

# Access in template
{{.references.config.data.setting}}
```

See the [Hardware References documentation](REFERENCES.md) for more details.

## Troubleshooting

### Secret Not Found

**Problem**: Workflow fails with "secret not found" error

**Solutions**:
- Verify the secret exists: `kubectl get secret <name> -n <namespace>`
- Check the secret name and namespace match exactly in `secretRef`
- Ensure the Tinkerbell controller has RBAC permissions

### Template Rendering Fails

**Problem**: Template renders with empty values or errors

**Solutions**:
- Verify secret keys exist: `kubectl get secret <name> -o yaml`
- Check template syntax: `.secret.<key>` (note the dot)
- Ensure secret data keys match what you're referencing

### Permission Denied

**Problem**: Controller can't access the secret

**Solutions**:
- Check RBAC permissions for the controller ServiceAccount
- Verify the controller is running in the correct namespace
- Review the controller logs for detailed error messages

## See Also

- [Hardware References](REFERENCES.md) - Linking to other Kubernetes resources
- [Kubernetes Secrets Documentation](https://kubernetes.io/docs/concepts/configuration/secret/)
- [Tinkerbell Templates](https://docs.tinkerbell.org/) - General template documentation
