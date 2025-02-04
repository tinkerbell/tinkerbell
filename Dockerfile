FROM scratch

COPY out/tinkerbell /tinkerbell
ENTRYPOINT [ "/tinkerbell" ]