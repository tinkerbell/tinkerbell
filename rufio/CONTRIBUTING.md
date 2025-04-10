# Contributor Guide

Welcome to Rufio! We are really excited to have you.
Please use the following guide on your contributing journey.
Thanks for contributing!

## Table of Contents

- [Context](#Context)
- [Prerequisites](#Prerequisites)
  - [DCO Sign Off](#DCO-Sign-Off)
  - [Code of Conduct](#Code-of-Conduct)
  - [Setting up your development environment](#Setting-up-your-development-environment)
- [Pull Requests](#Pull-Requests)
  - [Branching strategy](#Branching-strategy)
  - [Quality](#Quality)
    - [CI](#CI)
    - [Code coverage](#Code-coverage)
  - [Pre PR Checklist](#Pre-PR-Checklist)

---

## Context


Rufio is a Kubernetes controller for managing baseboard management state and actions.It is part of the [Tinkerbell stack](https://tinkerbell.org) and provides the glue for machine provisioning by enabling machine restarts and setting next boot devices.

## Prerequisites

### DCO Sign Off

Please read and understand the DCO found [here](docs/DCO.md).

### Code of Conduct

Please read and understand the code of conduct found [here](https://github.com/tinkerbell/rufio/blob/main/CODE_OF_CONDUCT.md).

### Setting up your development environment

1. Install Go

   Rufio requires [Go 1.17](https://golang.org/dl/) or later.

1. Install Docker

   Rufio uses Docker for protocol buffer code generation, container image builds and for the Ruby client example.
   Most versions of Docker will work.

> The items below are nice to haves, but not hard requirements for development

1. Install golangci-lint

   [golangci-lint](https://golangci-lint.run/usage/install/) is used in CI for lint checking and should be run locally before creating a PR.

## Pull Requests

### Branching strategy

Rufio uses a fork and pull request model.
See this [doc](https://guides.github.com/activities/forking/) for more details.

### Quality

#### CI

Rufio uses GitHub Actions for CI.
The workflow is found in [.github/workflows/ci.yaml](.github/workflows/ci.yaml).
It is run for each commit and PR.
The container image building only happens once a PR is merged into the main line.

#### Code coverage

Rufio does run code coverage with each PR.
Coverage thresholds are not currently enforced.
It is always nice and very welcomed to add tests and keep or increase the code coverage percentage.

### Pre PR Checklist

This checklist is a helper to make sure there's no gotchas that come up when you submit a PR.

- [ ] You've reviewed the [code of conduct](#Code-of-Conduct)
- [ ] All commits are DCO signed off
- [ ] Code is [formatted and linted](#Linting)
- [ ] Code [builds](#Building) successfully
- [ ] All tests are [passing](#Unit-testing)
- [ ] Code coverage [percentage](#Code-coverage). (main line is the base with which to compare)
