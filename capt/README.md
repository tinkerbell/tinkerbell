# Cluster API Provider Tinkerbell

[![Go Reference](https://pkg.go.dev/badge/github.com/tinkerbell/tinkerbell/capt.svg)](https://pkg.go.dev/github.com/tinkerbell/tinkerbell/capt)
[![CRD - reference](https://img.shields.io/badge/CRD-reference-2ea44f)](https://doc.crds.dev/github.com/tinkerbell/tinkerbell/capt)
[![Go Report Card](https://goreportcard.com/badge/github.com/tinkerbell/tinkerbell/capt)](https://goreportcard.com/report/github.com/tinkerbell/tinkerbell/capt)

<a href="https://kubernetes.io"><img src="https://github.com/kubernetes/kubernetes/raw/master/logo/logo.png"  height="100"></a>
<a href="https://tinkerbell.org"><img src="https://raw.githubusercontent.com/tinkerbell/artwork/main/Tinkerbell-Logo-Landscape-Dark.png" height="100"></a>

Kubernetes-native declarative infrastructure for Kubernetes clusters on Tinkerbell.

## What is the Cluster API Provider Tinkerbell

The [Cluster API][cluster_api] brings declarative, Kubernetes-style APIs to Kubernetes
cluster creation, configuration and management.

The API itself is shared across multiple cloud providers allowing for true hybrid
deployments of Kubernetes, both on-premises and off.

## Quick Start

See the [Quick Start](docs/QUICK-START.md)

## Compatibility with Cluster API and Kubernetes Versions

This provider's versions are compatible with the following versions of Cluster API:

|                                    | v1beta1 (v1.2) | v1beta1 (v1.6) |
| ---------------------------------- | -------------- | -------------- |
| Tinkerbell Provider v1beta1 (v0.4) | ✓              |                |
| Tinkerbell Provider v1beta1 (v0.5) |                | ✓              |

This provider's versions are able to install and manage the following versions of Kubernetes:

|                                     | v1.20 | v1.21 | v1.22 | v1.23 | v1.24 | v1.25 | v1.26 | v1.27 | v1.28 | v1.29 |
| ----------------------------------- | ----- | ----- | ----- | ----- | ----- | ----- | ----- | ----- | ----- | ----- |
| Tinkerbell Provider v1beta1 (v0.4)  | ✓     | ✓     | ✓     | ✓     |       |       |       |       |       |       |
| Tinkerbell Provider v1beta1 (v0.5)  |       |       |       |       |       | ✓     | ✓     | ✓     | ✓     | ✓     |

Each version of Cluster API for Tinkerbell will attempt to support all community supported Kubernetes versions during it's maintenance cycle; e.g., Cluster API for Tinkerbell `v0.5` supports Kubernetes 1.25, 1.26, 1.27, 1.28, 1.29.

**NOTE:** As the versioning for this project is tied to the versioning of Cluster API, future modifications to this
policy may be made to more closely align with other providers in the Cluster API ecosystem. See https://cluster-api.sigs.k8s.io/reference/versions

## Kubernetes versions with published Images

Pre-built images are pushed to the [GitHub Container Registry](https://github.com/orgs/tinkerbell/packages?repo_name=cluster-api-provider-tinkerbell). We currently publish images for [Ubuntu 18.04](https://github.com/tinkerbell/tinkerbell/capt/pkgs/container/cluster-api-provider-tinkerbell%2Fubuntu-1804) and [Ubuntu 20.04](https://github.com/tinkerbell/tinkerbell/capt/pkgs/container/cluster-api-provider-tinkerbell%2Fubuntu-2004).

## Current state

Currently, it is possible to bootstrap both single instance and multiple instance Control Plane
workload clusters using hardware managed by Tinkerbell.

See [docs/README.md](docs/README.md) for more information on setting up a development
environment.

<!-- links -->
[cluster_api]: https://cluster-api.sigs.k8s.io
