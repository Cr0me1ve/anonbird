# build & run locally with:
#   cd "$(git rev-parse --show-toplevel)"
#   CGO_ENABLED=0 go build -o anonbird ./client
#   sudo podman build -t localhost/anonbird:latest -f client/Dockerfile --ignorefile .dockerignore-client .
#   sudo podman run --rm -it --cap-add={BPF,NET_ADMIN,NET_RAW} localhost/anonbird:latest

FROM alpine:3.23.3
# iproute2: busybox doesn't display ip rules properly
RUN apk add --no-cache \
    bash \
    ca-certificates \
    ip6tables \
    iproute2 \
    iptables

ENV \
    ANONBIRD_BIN="/usr/local/bin/anonbird" \
    NB_LOG_FILE="console,/var/log/anonbird/client.log" \
    NB_DAEMON_ADDR="unix:///var/run/anonbird.sock" \
    NB_ENABLE_CAPTURE="false" \
    NB_ENTRYPOINT_SERVICE_TIMEOUT="30"

ENTRYPOINT [ "/usr/local/bin/anonbird-entrypoint.sh" ]

ARG ANONBIRD_BINARY=anonbird
COPY client/anonbird-entrypoint.sh /usr/local/bin/anonbird-entrypoint.sh
COPY "${ANONBIRD_BINARY}"  /usr/local/bin/anonbird
