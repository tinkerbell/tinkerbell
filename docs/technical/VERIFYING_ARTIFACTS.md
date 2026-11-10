# Verifying release artifacts

CI signs every artifact it publishes, with keyless [Sigstore](https://www.sigstore.dev/) signatures tied to the workflow that built it. There are no keys to fetch.

| Artifact | Signed with | SBOM |
| --- | --- | --- |
| `ghcr.io/tinkerbell/tinkerbell`, `ghcr.io/tinkerbell/tink-agent` | cosign | SPDX, attached to the image by BuildKit |
| `oci://ghcr.io/tinkerbell/charts/tinkerbell` | cosign | none |
| `tinkerbell-embedded-linux-{amd64,arm64}` release assets | GitHub build provenance attestation | SPDX, `<binary>.spdx.json` release asset |
| `<binary>.spdx.json` and `checksums-embedded.txt` release assets | GitHub build provenance attestation | none |

The first signed release is `v0.26.0`. Earlier releases are not signed.

Releases are signed by `https://github.com/tinkerbell/tinkerbell/.github/workflows/release.yaml@refs/tags/<version>`. Builds of `main` are signed by `https://github.com/tinkerbell/tinkerbell/.github/workflows/ci.yaml@refs/heads/main`. Check the identity that matches what you deploy: a `main` build does not verify as a release.

Requirements:

- [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v3 or later. cosign v2.6 and later also work with `--new-bundle-format`.
- [gh](https://cli.github.com/), logged in with `gh auth login`, for the release binaries.
- `docker buildx`, to read the image SBOMs.
- Flux v2.8.0 or later, to verify the chart from Flux.

The examples use:

```sh
VERSION=v0.26.0  # the release to check
```

## Container images

```sh
cosign verify "ghcr.io/tinkerbell/tinkerbell:${VERSION}" \
  --certificate-identity "https://github.com/tinkerbell/tinkerbell/.github/workflows/release.yaml@refs/tags/${VERSION}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

The same works for `ghcr.io/tinkerbell/tink-agent`. The multi-platform index and each platform manifest are signed, so a reference pinned to either digest verifies too.

Deploy the digest you verified, not the tag: a tag can move after you check it. `cosign verify` prints the digest it checked:

```sh
DIGEST=$(cosign verify "ghcr.io/tinkerbell/tinkerbell:${VERSION}" \
  --certificate-identity "https://github.com/tinkerbell/tinkerbell/.github/workflows/release.yaml@refs/tags/${VERSION}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  | jq -r '.[0].critical.image."docker-manifest-digest"')
echo "ghcr.io/tinkerbell/tinkerbell@${DIGEST}"
```

### Image SBOMs

Read the SBOM from the index digest you verified, so it belongs to that image. The SBOM hangs off the index, so a platform manifest digest doesn't reach it: the command then prints `{}`. `cosign verify` on a tag prints the index digest.

```sh
docker buildx imagetools inspect "ghcr.io/tinkerbell/tinkerbell@${DIGEST}" --format '{{ json .SBOM }}'
```

The output holds one SPDX document per platform. `cosign verify-attestation` does not see these SBOMs: BuildKit references them from the image index, which the image signature covers.

### `:latest` and mainline builds

`:latest` moves between mainline and release builds, so its signer is either `ci.yaml` at `refs/heads/main` or `release.yaml` at `refs/tags/vX.Y.Z`:

```sh
cosign verify ghcr.io/tinkerbell/tinkerbell:latest \
  --certificate-identity-regexp '^https://github\.com/tinkerbell/tinkerbell/\.github/workflows/(ci\.yaml@refs/heads/main|release\.yaml@refs/tags/v[^/]+)$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Mainline builds are also tagged `vX.Y.(Z+1)-<short-commit-hash>`, as described in [Releasing](../RELEASING.md#mainline-releases). Verify those with `--certificate-identity "https://github.com/tinkerbell/tinkerbell/.github/workflows/ci.yaml@refs/heads/main"`.

Mainline builds reuse CI build caches, which release builds skip. In production, deploy a release and pin its identity.

## Helm chart

```sh
cosign verify "ghcr.io/tinkerbell/charts/tinkerbell:${VERSION}" \
  --certificate-identity "https://github.com/tinkerbell/tinkerbell/.github/workflows/release.yaml@refs/tags/${VERSION}" \
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
    tag: v0.26.0
  layerSelector:
    mediaType: application/vnd.cncf.helm.chart.content.v1.tar+gzip
    operation: copy
  verify:
    provider: cosign
    matchOIDCIdentity:
      - issuer: ^https://token\.actions\.githubusercontent\.com$
        subject: ^https://github\.com/tinkerbell/tinkerbell/\.github/workflows/release\.yaml@refs/tags/v0\.26\.0$
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

Flux `verify` checks only the chart, not the images the chart deploys. Verify those as shown in [Container images](#container-images).

## Release binaries

```sh
gh release download "${VERSION}" --repo tinkerbell/tinkerbell

gh attestation verify tinkerbell-embedded-linux-amd64 \
  --repo tinkerbell/tinkerbell \
  --cert-identity "https://github.com/tinkerbell/tinkerbell/.github/workflows/release.yaml@refs/tags/${VERSION}" \
  --deny-self-hosted-runners
```

Run `gh attestation verify` for each file you use, including `checksums-embedded.txt` and the `.spdx.json` SBOMs.
