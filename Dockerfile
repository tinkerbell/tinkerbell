FROM alpine:3.21

# Install ipmitool needed for bmclib.
RUN apk add --upgrade ipmitool=1.8.19-r1

COPY out/tinkerbell /tinkerbell
ENTRYPOINT [ "/tinkerbell" ]