# Build the manager binary
FROM golang:1.18 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY ./ ./

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager main.go

FROM alpine:3.8

ARG IPMITOOL_REPO=https://github.com/ipmitool/ipmitool.git
ARG IPMITOOL_COMMIT=IPMITOOL_1_8_18

# Install dependencies for ipmitool
WORKDIR /tmp
RUN apk add --update --upgrade --no-cache --virtual build-deps \
        alpine-sdk=1.0-r0 \
        autoconf=2.69-r2 \
        automake=1.16.1-r0 \
        git=2.18.4-r0 \
        libtool=2.4.6-r5 \
        ncurses-dev=6.1_p20180818-r1 \
        openssl-dev=1.0.2u-r0 \
        readline-dev=7.0.003-r0 \
    && apk add --update --upgrade --no-cache --virtual run-deps \
	    ca-certificates=20191127-r2 \
        libcrypto1.0=1.0.2u-r0 \
        musl=1.1.19-r11 \
        readline=7.0.003-r0 \
    && git clone -b master ${IPMITOOL_REPO}

# Install ipmitool
WORKDIR /tmp/ipmitool
RUN git checkout ${IPMITOOL_COMMIT} \
    && ./bootstrap \
    && ./configure \
        --prefix=/usr/local \
        --enable-ipmievd \
        --enable-ipmishell \
        --enable-intf-lan \
        --enable-intf-lanplus \
        --enable-intf-open \
    && make \
    && make install \
    && apk del build-deps

WORKDIR /tmp
RUN rm -rf /tmp/ipmitool

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
