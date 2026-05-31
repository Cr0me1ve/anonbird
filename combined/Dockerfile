FROM ubuntu:24.04
RUN apt update && apt install -y ca-certificates && rm -fr /var/cache/apt
ENTRYPOINT [ "/go/bin/anonbird-server" ]
CMD ["--config", "/etc/anonbird/config.yaml"]
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/anonbird-server /go/bin/anonbird-server
