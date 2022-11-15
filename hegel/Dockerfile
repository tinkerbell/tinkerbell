FROM alpine:3.7

# Define args for the target platform so we can identify the binary in the Docker context.
# These args are populated by Docker. The values should match Go's GOOS and GOARCH values for
# the respective os/platform.
ARG TARGETARCH
ARG TARGETOS

RUN apk add --update --upgrade ca-certificates

RUN adduser -D -u 1000 tinkerbell
USER tinkerbell

COPY --chown=tinkerbell ./hegel-$TARGETOS-$TARGETARCH /usr/bin/hegel
ENTRYPOINT ["/usr/bin/hegel"]
