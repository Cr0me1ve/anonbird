FROM gcr.io/distroless/base:debug
ENTRYPOINT [ "/go/bin/anonbird-upload" ]
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/anonbird-upload /go/bin/anonbird-upload
