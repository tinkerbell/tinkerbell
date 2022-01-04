[![Test and Build](https://github.com/tinkerbell/ipxedust/actions/workflows/ci.yaml/badge.svg)](https://github.com/tinkerbell/ipxedust/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/tinkerbell/ipxedust/branch/main/graph/badge.svg)](https://codecov.io/gh/tinkerbell/ipxedust)
[![Go Report Card](https://goreportcard.com/badge/github.com/tinkerbell/ipxedust)](https://goreportcard.com/report/github.com/tinkerbell/ipxedust)
[![Go Reference](https://pkg.go.dev/badge/github.com/tinkerbell/ipxedust.svg)](https://pkg.go.dev/github.com/tinkerbell/ipxedust)

# ipxedust

TFTP and HTTP library and cli for serving [iPXE](https://ipxe.org/) binaries.

## Build

```bash
make build
```

## Usage

CLI

```bash
./bin/ipxe-linux -h # ./bin/ipxe-darwin -h

USAGE
  Run TFTP and HTTP iPXE binary server

FLAGS
  -http-addr 0.0.0.0:8080  HTTP server address
  -http-timeout 5s         HTTP server timeout
  -log-level info          Log level
  -tftp-addr 0.0.0.0:69    TFTP server address
  -tftp-timeout 5s         TFTP server timeout

```

## Design Philosophy

This repository is designed to be both a library and a command line tool.
The custom iPXE binaries are built in the open. See the iPXE doc [here](docs/IPXE.md) for details.
The coding design philosophy can be found [here](docs/Philosophy.md).

## System Context Diagram

The following diagram details how `ipxedust`(ipxe binaries) fits into the greater Boots(PXE) stack. [Architecture](docs/architecture.png).
