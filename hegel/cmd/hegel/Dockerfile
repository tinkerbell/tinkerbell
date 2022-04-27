FROM golang:1.17.5-alpine AS builder

ARG GOPROXY=https://proxy.golang.org,direct

RUN apk add --update --upgrade git make
WORKDIR /src
ENV GOPROXY=$GOPROXY

# Copy only the go mod files so we can take advantage of image layer caching.
COPY go.* ./
RUN go mod download

COPY . .
RUN make

FROM alpine:3.7

EXPOSE 50060
EXPOSE 50061
ENTRYPOINT ["/usr/bin/hegel"]

RUN apk add --update --upgrade ca-certificates
RUN adduser -D -u 1000 tinkerbell
USER tinkerbell

COPY --from=builder /src/hegel /usr/bin/hegel
