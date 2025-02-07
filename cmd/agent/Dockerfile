FROM golang:1.23.2-alpine3.20 AS builder

WORKDIR /code
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o tink-agent

FROM scratch
COPY --from=builder /code/tink-agent /tink-agent

ENTRYPOINT ["/tink-agent"]