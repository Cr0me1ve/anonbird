FROM gcr.io/distroless/base:debug
ENTRYPOINT [ "/go/bin/anonbird-signal","run" ]
CMD ["--log-file", "console"]
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/anonbird-signal /go/bin/anonbird-signal
