# Rufio

![For each commit and PR](https://github.com/tinkerbell/rufio/workflows/For%20each%20commit%20and%20PR/badge.svg)
![stability](https://img.shields.io/badge/Stability-Experimental-red.svg)

> This repository is [Experimental](https://github.com/packethost/standards/blob/main/experimental-statement.md) meaning that it's based on untested ideas or techniques and not yet established or finalized or involves a radically new and innovative style!
> This means that support is best effort (at best!) and we strongly encourage you to NOT use this in production.

## Description

**R**ufio
**U**ses
**F**ancy
**I**PMI
**O**perations

Rufio is a Kubernetes controller for managing baseboard management state and actions.

Goals:

* Declaratively enforce and ensure BMC state such as:
  * Power state
  * Persistent boot order
  * NTP/LDAP/TLS Cert configuration (probably future scope)
 * Declaratively enact actions such as:
   *  Power reset
   * Configure ephemeral boot order
* Report state such as:
  * Firmware version
  * Other Redfish/Swordfish machine metadata

Implementation
* Kubernetes-based controller
* AuthN/Z is enforced with Kubernetes Authentication/RBAC
* BMC authentication is managed with Kubernetes Secrets

## Contributing

See the contributors guide [here](CONTRIBUTING.md).

## Website

For complete documentation, please visit the Tinkerbell project hosted at [tinkerbell.org](https://tinkerbell.org).
