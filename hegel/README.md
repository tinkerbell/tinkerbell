[![Build Status](https://cloud.drone.io/api/badges/packethost/hegel/status.svg)](https://cloud.drone.io/packethost/hegel)

### Hegel
The logical successor to Kant? A gRPC metadata service for Tinkerbell. Subscribe to changes in device metadata, get notified when IPs are added/removed, a device appears in the project, spot instance termination is triggered, etc.


#### Notes

`protoc -I ./protos/hegel ./protos/hegel/hegel.proto --go_out=plugins=grpc:./protos/hegel`


#### Create and use self signed certificates

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

#### Running Hegel Locally

The `docker-compose.yml` in the root of this repo makes it possible to run `hegel` locally for verifying basic functionality.
There are a number of env vars you'll want to set before running `docker-compose`:

- `FACILITY` - facility code
- `PACKET_API_AUTH_TOKEN` - an API token
- `PACKET_API_URL` - `https://api.packet.net/` for using the production API
