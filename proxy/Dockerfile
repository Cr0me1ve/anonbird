FROM golang:1.25-alpine AS builder
WORKDIR /app

RUN echo "anonbird:x:1000:1000:anonbird:/var/lib/anonbird:/sbin/nologin" > /tmp/passwd && \
    echo "anonbird:x:1000:anonbird" > /tmp/group && \
    mkdir -p /tmp/var/lib/anonbird && \
    mkdir -p /tmp/certs

FROM gcr.io/distroless/base:debug
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/anonbird-proxy /go/bin/anonbird-proxy
COPY --from=builder /tmp/passwd /etc/passwd
COPY --from=builder /tmp/group /etc/group
COPY --from=builder --chown=1000:1000 /tmp/var/lib/anonbird /var/lib/anonbird
COPY --from=builder --chown=1000:1000 --chmod=755 /tmp/certs /certs
USER anonbird:anonbird
ENV HOME=/var/lib/anonbird
ENV NB_PROXY_ADDRESS=":8443"
EXPOSE 8443
ENTRYPOINT ["/go/bin/anonbird-proxy"]
