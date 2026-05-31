FROM gcr.io/distroless/base:debug
ENTRYPOINT [ "/go/bin/anonbird-relay" ]
ENV NB_LOG_FILE=console
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/anonbird-relay /go/bin/anonbird-relay
