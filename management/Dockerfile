FROM ubuntu:24.04
RUN apt update && apt install -y ca-certificates && rm -fr /var/cache/apt
ENTRYPOINT [ "/go/bin/anonbird-mgmt","management"]
CMD ["--log-file", "console"]
COPY anonbird-mgmt /go/bin/anonbird-mgmt
