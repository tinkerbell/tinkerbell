[![Test and Build](https://github.com/tinkerbell/boots-ipxe/actions/workflows/ci.yaml/badge.svg)](https://github.com/tinkerbell/boots-ipxe/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/tinkerbell/boots-ipxe/branch/main/graph/badge.svg)](https://codecov.io/gh/tinkerbell/boots-ipxe)
[![Go Report Card](https://goreportcard.com/badge/github.com/tinkerbell/boots-ipxe)](https://goreportcard.com/report/github.com/tinkerbell/boots-ipxe)
[![Go Reference](https://pkg.go.dev/badge/github.com/tinkerbell/boots-ipxe.svg)](https://pkg.go.dev/github.com/tinkerbell/boots-ipxe)

# boots-ipxe

TFTP and HTTP library and cli for serving [iPXE](https://ipxe.org/) binaries.

## Design Philosophy

This repository is designed to be both a library and a command line tool.
The custom iPXE binaries are built in the open. See the iPXE doc [here](docs/IPXE.md) for details.

## System Context Diagram

The following diagram details how `boots-ipxe`(ipxe binaries) fits into the greater Boots(PXE) stack. [Architecture](docs/architecture.png).
