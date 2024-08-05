FROM alpine:3.20.2

# Define args for the target platform so we can identify the binary in the Docker context.
# These args are populated by Docker. The values should match Go's GOOS and GOARCH values for
# the respective os/platform.
ARG TARGETARCH
ARG TARGETOS

# hadolint ignore=DL3018
RUN apk add --update --upgrade --no-cache ca-certificates && \
    adduser -D -u 1000 tinkerbell

COPY ./hegel-$TARGETOS-$TARGETARCH /usr/bin/hegel

# Switching to the tinkerbell user should be done as late as possible so we still use root to
# perform the other commands.
USER tinkerbell
ENTRYPOINT ["/usr/bin/hegel"]
