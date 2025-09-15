# Contributor Guide

Welcome to Tinkerbell!
We are really excited to have you.
Please use the following guide on your contributing journey.
Thanks for contributing!

## Table of Contents

- [Context](#context)
- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
  - [DCO Sign Off](#dco-sign-off)
  - [Code of Conduct](#code-of-conduct)
- [Development](#development)
  - [Setting up your development environment](#setting-up-your-development-environment)
  - [Building](#building)
  - [Unit testing](#unit-testing)
  - [Linting](#linting)
  - [CI](#ci)
- [Pull Requests](#pull-requests)
  - [Branching strategy](#branching-strategy)
  - [Quality](#quality)
    - [Code coverage](#code-coverage)
  - [Pre PR Checklist](#pre-pr-checklist)

---

## Context

This document is a guide for contributing to Tinkerbell.

## Architecture

The Tinkerbell project is a collection of components that work together to provide bare metal provisioning.
Each component is its own Go package. Care should be taken to keep the packages isolated from dependency outside of themselves.
To aid in this, a dependency graph can be generated with the following command, `make dep-graph`.
The following components have there own packages and are top level directories in the repository.

- `helm` - The Helm chart.
- `smee` - The network boot server.
- `tink/agent` - The Agent.
- `tink/server` - The Server.
- `tink/controller` - The Workflow controller.
- `tootles` - The metadata service.
- `secondstar` - The serial over SSH service.
- `cmd` - The command line code.
- `crd` - The code for handling custom resource definitions.
- `pkg` - Packages that are shared between components (Extra careful consideration should be used when adding packages here).
- `apiserver` - The embedded Kubernetes API server code, this is only included when explicitly enabled, not included by default.

## Prerequisites

### DCO Sign Off

Please read and understand the DCO found [here](https://github.com/tinkerbell/org/blob/main/DCO.md).

### Code of Conduct

Please read and understand the code of conduct found [here](https://github.com/tinkerbell/.github/blob/main/CODE_OF_CONDUCT.md).

## Development

### Setting up your development environment

The following are expected to be installed on your development machine.

- Go 1.25 or later
- Docker
- Make
- Git
- Helm

### Building

There are `Makefile` targets for building the Tinkerbell components.
These components include, the Tinkerbell server binary, the Tinkerbell agent binary, the Tinkerbell container image, the Tinkerbell agent container image, and the Tinkerbell Helm chart. The binaries and container images can be built for multiple architectures. To see the available `make` targets for building components, run `make help`.

### Unit testing

Before opening a Pull Request, please ensure that unit tests are passing. Run the unit tests locally with, `make test`.

### Linting

Before opening a Pull Request, please ensure that the code is formatted and linted. Run the linter locally with, `make lint`.

## CI

To run the same `make` targets that are run in CI, run the following command, `make ci`.

## Pull Requests

### Branching strategy

Smee uses a fork and pull request model.
See this [doc](https://guides.github.com/activities/forking/) for more details.

### Quality

#### Code coverage

Tinkerbell runs code coverage with each PR.
Code coverage is reported in the CI output and on the PR.
All PR's should have a total coverage difference of less than 1% from the main branch.

### Pre PR Checklist

This checklist is a helper to make sure there's no gotchas that come up when you submit a PR.

- [ ] You've reviewed the [code of conduct](#code-of-conduct)
- [ ] All commits are DCO signed off
- [ ] Code is [formatted and linted](#linting)
- [ ] Code [builds](#building) successfully
- [ ] All tests are [passing](#unit-testing)
- [ ] Code coverage [percentage](#code-coverage). (main line is the base with which to compare)
