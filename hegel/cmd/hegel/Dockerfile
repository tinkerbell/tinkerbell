FROM golang:1.15-alpine
COPY . /usr/myapp
WORKDIR /usr/myapp
RUN mkdir -p bin && go build -o ./bin ./...

FROM alpine:3.7

EXPOSE 50060
EXPOSE 50061
ENTRYPOINT ["/usr/bin/hegel"]

RUN apk add --update --upgrade ca-certificates
RUN adduser -D -u 1000 tinkerbell
USER tinkerbell

COPY --from=0 /usr/myapp/bin/hegel /usr/bin/hegel
