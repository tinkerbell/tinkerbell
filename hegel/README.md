# Hegel

[![Build status](https://img.shields.io/github/workflow/status/tinkerbell/hegel/Hegel?label=Build&logo=github)](https://img.shields.io/github/workflow/status/tinkerbell/hegel/Hegel?label=Hegel&logo=github) 
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

#### Version Compatibility

We follow semantic versioning and the project is currently v0 meaning compatibility is best effort.
If you have any specific concerns don't hesitate to raise an issue.

## How does it work?

When Hegel receives an HTTP request it inspects the request source IP address and tries to find a
matching instance using its configured backend. If an instance is found, it serves the data for the
requested path.

If no instance data matching the source IP was found it returns a 404 Not Found.

## FAQ

### How do I impersonate an instance?

Sometimes its necessary to impersonate an instance so you can `curl` or otherwise debug what data 
Hegel is serving. Hegel offers a `--trusted-proxies` CLI option (configurable as an env var with
`HEGEL_TRUSTED_PROXIES`) that lets you specify your host IP address as trusted. Trusted IPs can
submit requests with the `X-Forwarded-For` header set to the IP they wish to impersonate.

**Example**

```sh
# Launch Hegel with the localhost address as trusted. Note this isn't a functional command 
# as it doesn't instruct Hegel on what backend to use.
docker run -d -e HEGEL_TRUSTED_PROXIES="127.0.0.1" quay.io/tinkerbell/hegel:v0
```

```sh
# cURL an endpoint specifying what address you're impersonating.
curl -H "X-Forwarded-For: 10.10.10.10" http://localhost:50061/2009-04-04/meta-data/hostname
```


[cloud-init]: https://cloudinit.readthedocs.io/en/latest/
[ignition]: https://coreos.github.io/ignition/