[![Build Status](https://cloud.drone.io/api/badges/tinkerbell/hegel/status.svg)](https://cloud.drone.io/tinkerbell/hegel)

### Hegel
The logical successor to Kant? The gRPC and HTTP metadata service for Tinkerbell. Subscribe to changes in metadata, get notified when data is added/removed, etc.

Full documentation can be found at [tinkerbell.org](https://github.com/tinkerbell/tink)


#### Notes

`protoc -I ./protos/hegel ./protos/hegel/hegel.proto --go_out=plugins=grpc:./protos/hegel`


#### Self-Signed Certificates

To use Hegel with TLS certificates:

    mkdir ./certs
    pushd ./certs
    openssl genrsa -des3 -passout pass:x -out server.pass.key 2048
    openssl rsa -passin pass:x -in server.pass.key -out server.key
    openssl req -new -key server.key -out server.csr
    openssl x509 -req -sha256 -days 365 -in server.csr -signkey server.key -out server.crt
    popd

    export HEGEL_TLS_CERT=certs/server.crt
    export HEGEL_TLS_KEY=certs/server.key
    go run main.go
