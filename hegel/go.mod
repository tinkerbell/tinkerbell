module github.com/packethost/hegel

go 1.13

require (
	github.com/golang/protobuf v1.4.2
	github.com/grpc-ecosystem/go-grpc-middleware v1.2.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/packethost/cacher v0.0.0-20200319200613-5dc1cac4fd33
	github.com/packethost/pkg v0.0.0-20190715213007-7c3a64b4b5e3
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.5.1
	github.com/tinkerbell/tink v0.0.0-20200708073808-997394055483
	google.golang.org/grpc v1.29.1
)

replace github.com/tinkerbell/tink v0.0.0-20200622174048-5e66098170f3 => github.com/kdeng3849/tink v0.0.0-20200706184442-ed174c5a9d20
