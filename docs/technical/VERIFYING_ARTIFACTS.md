# Verifying release artifacts

CI signs every artifact it publishes, with keyless [Sigstore](https://www.sigstore.dev/) signatures tied to the workflow that built it. There are no keys to fetch.

| Artifact | Signed with |
| --- | --- |
| `ghcr.io/tinkerbell/tinkerbell`, `ghcr.io/tinkerbell/tink-agent` | cosign |
| `oci://ghcr.io/tinkerbell/charts/tinkerbell` | cosign |
| `tinkerbell-embedded-linux-{amd64,arm64}` release assets | GitHub build provenance attestation |

Releases are signed by `https://github.com/tinkerbell/tinkerbell/.github/workflows/ci.yaml@refs/tags/<version>`. Builds of `main` are signed by `...ci.yaml@refs/heads/main`. Check the identity that matches what you deploy: a `main` build does not verify as a release.

Requirements:

- [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v3 or later. cosign v2.6 and later also work with `--new-bundle-format`.
- [gh](https://cli.github.com/), logged in with `gh auth login`, for the release binaries.
- Flux v2.8.0 or later, to verify the chart from Flux.

The examples use:

```sh
VERSION=v0.25.1  # the release to check
```

## Container images

```sh
cosign verify "ghcr.io/tinkerbell/tinkerbell:${VERSION}" \
  --certificate-identity "https://github.com/tinkerbell/tinkerbell/.github/workflows/ci.yaml@refs/tags/${VERSION}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

The same works for `ghcr.io/tinkerbell/tink-agent`. The multi-platform index and each platform manifest are signed, so a reference pinned to either digest verifies too.

## Helm chart

```sh
cosign verify "ghcr.io/tinkerbell/charts/tinkerbell:${VERSION}" \
  --certificate-identity "https://github.com/tinkerbell/tinkerbell/.github/workflows/ci.yaml@refs/tags/${VERSION}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

With Flux:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: tinkerbell
  namespace: tinkerbell
spec:
  interval: 1h
  url: oci://ghcr.io/tinkerbell/charts/tinkerbell
  ref:
    tag: v0.25.1
  layerSelector:
    mediaType: application/vnd.cncf.helm.chart.content.v1.tar+gzip
    operation: copy
  verify:
    provider: cosign
    matchOIDCIdentity:
      - issuer: ^https://token\.actions\.githubusercontent\.com$
        subject: ^https://github\.com/tinkerbell/tinkerbell/\.github/workflows/ci\.yaml@refs/tags/v0\.25\.1$
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: tinkerbell
  namespace: tinkerbell
spec:
  interval: 1h
  chartRef:
    kind: OCIRepository
    name: tinkerbell
```

Update `ref.tag` and `subject` together on upgrades. A `subject` ending in `refs/tags/v.+$` works with `ref.semver`, but then accepts an older signed chart retagged as a newer version.

## Release binaries

```sh
gh release download "${VERSION}" --repo tinkerbell/tinkerbell

gh attestation verify tinkerbell-embedded-linux-amd64 \
  --repo tinkerbell/tinkerbell \
  --signer-workflow tinkerbell/tinkerbell/.github/workflows/ci.yaml \
  --source-ref "refs/tags/${VERSION}"
```

Run `gh attestation verify` for each file you use.
