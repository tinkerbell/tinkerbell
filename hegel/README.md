# Hegel

[![Build status](https://img.shields.io/github/actions/workflow/status/tinkerbell/hegel/ci.yaml?branch=main)](https://img.shields.io/github/actions/workflow/status/tinkerbell/hegel/ci.yaml?branch=main) 
[![Go version](https://img.shields.io/github/go-mod/go-version/tinkerbell/hegel?logo=go)](https://img.shields.io/github/go-mod/go-version/tinkerbell/hegel)
[![slack](https://img.shields.io/badge/CNCF-%23tinkerbell-blue?logo=slack)](https://cloud-native.slack.com/archives/C01SRB41GMT)
[![Docker images](https://img.shields.io/badge/Image-quay.io/tinkerbell/hegel-blue?logo=docker)](https://quay.io/repository/tinkerbell/hegel?tab=tags)

Hegel is an instance metadata service used by Tinkerbell for bare metal instance initialization.

When bare metal machines are provisioned using the Tinkerbell stack they inevitably boot into a
permanent OS. The permanent OS, much like the underlying hardware, needs initializing before it 
can be used. The initialization is commonly performed by tools such as [cloud-init] or [ignition]. 
The configuration used by these processes is provided by an instance metadata source. The source
could be anything but is commonly an HTTP API.

Hegel exposes common instance metadata APIs for your OS intialization needs including AWS EC2 
instance metadata.

## How does it work?

When Hegel receives an HTTP request it inspects the request source IP address and tries to find a
matching instance using its configured backend. If an instance is found, it serves the data for the
requested path. If no instance data matching the source IP was found it returns a 404 Not Found.

## Releases

Hegel releases with [semantic versioning v2][semver]. Each release produces 3 image tags using major (M) 
minor (m) and patch (p) numbers: `vM.m.p`, `vM.m` and `vM`. The `vM` will always point
to the latest minor release. `vM.m` will always point to the latest patch release.

For information on how to create a release, see [RELEASING.md][releasing].

### Version Compatibility

The project is currently v0 meaning compatibility is best effort. If you have any specific concerns 
do not hesitate to raise an issue.

## Quick Start

**Pre-requisits**
- Make
- Go
- Docker with BuildKit

```sh
# Build a Docker image for the host platform.
$ make image

# To test the image see the "How to impersonate an instance?" FAQ to launch Hegel. Ensure you use
# hegel:latest as the image name to use the newly built image.
```

See ["How do I Impersonate an Instance?"](#how-do-i-impersonate-an-instance) to launch Hegel and
ensure you use the `hegel:latest` image.

## Contributing

See [CONTRIBUTING.md](/CONTRIBUTING.md).

## FAQ

### What does variable X accept?

All variables are explained in the help output of Hegel.

```
docker run --rm quay.io/tinkerbell/hegel:latest -h
```

### How do I impersonate an instance?

Sometimes its necessary to impersonate an instance so you can `curl` or otherwise debug what data 
Hegel is serving. Hegel offers a `--trusted-proxies` CLI option (configurable as an env var with
`HEGEL_TRUSTED_PROXIES`) that lets you specify your host IP address as trusted. Trusted IPs can
submit requests with the `X-Forwarded-For` header set to the IP they wish to impersonate.

**Example**

```sh
# Launch Hegel trusting the Docker default gateway so we can impersonate machines.
#
# The trusted proxy 0.0.0.0/0 causes Hegel to trust all requesters. The sample flatfile.yml is
# configured to output success messages on when the API calls are successful.
#
# If the container doesn't launch and there's no `docker run` logging remove the --rm flag 
# so the container remains on disk and can be inspected with `docker logs`.
docker run --rm -d --name=hegel \
    -p 50061:50061 \
    -v $PWD/samples/flatfile.yml:/flatfile.yml \
    -e HEGEL_TRUSTED_PROXIES="0.0.0.0/0" \
    -e HEGEL_BACKEND="flatfile" \
    -e HEGEL_FLATFILE_PATH="/flatfile.yml" \
    quay.io/tinkerbell/hegel:latest
```

```sh
# cURL an endpoint specifying what address you're impersonating.
# Expected response:
# Success! You retrieved the hostname
curl -H "X-Forwarded-For: 10.10.10.10" http://localhost:50061/2009-04-04/meta-data/hostname
```

### What is the difference between `/metadata` and `/2009-04-04/meta-data`?

The `/metadata` endpoint historically servced [Equinix Metal metadata][equinix-metadata]. It has 
since been reduced to the minimum required to satisfy known [Tinkerbell Hub Actions][hub] that
rely on the data to perform their function.

The `/2009-04-04/meta-data` endpoint is an [EC2 Instance Metadata][ec2-im] endpoint that servces a set of
additional endpoints that can be queried for data. The EC2 Instance Metadata support Hegel provides
enables integration with other tooling.

[cloud-init]: https://cloudinit.readthedocs.io/en/latest/
[ignition]: https://coreos.github.io/ignition/
[releasing]: /RELEASING.md
[frontend-backend]: /docs/design/frontend-backend.puml
[semver]: https://semver.org/
[equinix-metadata]: https://deploy.equinix.com/developers/docs/metal/server-metadata/metadata/
[hub]: https://github.com/tinkerbell/hub
[ec2-im]: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instancedata-data-categories.html