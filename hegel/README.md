### Hegel

[![Build Status](https://github.com/tinkerbell/hegel/workflows/For%20each%20commit%20and%20PR/badge.svg)](https://github.com/tinkerbell/hegel/actions?query=workflow%3A%22For+each+commit+and+PR%22+branch%3Amaster)
![](https://img.shields.io/badge/Stability-Experimental-red.svg)

This repository is [Experimental](https://github.com/packethost/standards/blob/master/experimental-statement.md) meaning that it's based on untested ideas or techniques and not yet established or finalized or involves a radically new and innovative style! This means that support is best effort (at best!) and we strongly encourage you to NOT use this in production.

The gRPC and HTTP metadata service for Tinkerbell.
Subscribe to changes in metadata, get notified when data is added/removed, etc.

Full documentation can be found at [tinkerbell.org](https://github.com/tinkerbell/tink)


#### Notes

`protoc -I ./protos/hegel ./protos/hegel/hegel.proto --go_out=plugins=grpc:./protos/hegel`


#### Self-Signed Certificates

To use Hegel with TLS certificates:

```shell
mkdir ./certs
openssl genrsa -des3 -passout pass:x -out ./certs/server.pass.key 2048
openssl rsa -passin pass:x -in ./certs/server.pass.key -out ./certs/server.key
openssl req -new -key ./certs/server.key -out ./certs/server.csr
openssl x509 -req -sha256 -days 365 -in ./certs/server.csr -signkey ./certs/server.key -out ./certs/server.crt
export HEGEL_TLS_CERT=./certs/server.crt
export HEGEL_TLS_KEY=./certs/server.key
go run main.go
```
