binary := cmd/hegel
all: ${binary}
.PHONY: ${binary} gen test
${binary}: grpc/protos/hegel
${binary}:
	CGO_ENABLED=0 GOOS=$$GOOS go build -ldflags="-X main.GitRev=$(shell git rev-parse --short HEAD)" -o $@ ./$@
gen: grpc/protos/hegel/hegel.pb.go
grpc/protos/hegel/hegel.pb.go: grpc/protos/hegel/hegel.proto
	protoc --go_out=plugins=grpc:./ grpc/protos/hegel/hegel.proto
	goimports -w $@
ifeq ($(CI),drone)
run: ${binary}
	${binary}
test:
	go test -race -coverprofile=coverage.txt -covermode=atomic ${TEST_ARGS} ./...
endif
