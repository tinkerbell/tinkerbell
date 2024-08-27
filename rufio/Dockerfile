# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.23 AS builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY ./ ./

# Define args for the target platform so we can identify the binary in the Docker context.
# These args are populated by Docker. The values should match Go's GOOS and GOARCH values for
# the respective os/platform.
ARG TARGETARCH
ARG TARGETOS

# Build
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -a -o manager main.go

FROM alpine:3.20

# Install ipmitool required by the third party BMC lib.
RUN apk add --upgrade ipmitool=1.8.19-r1

COPY --from=builder /workspace/manager .

USER 65532:65532
ENTRYPOINT ["/manager"]
