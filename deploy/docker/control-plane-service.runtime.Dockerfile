FROM scratch

COPY bin/linux/control-plane-service /control-plane-service

ENTRYPOINT ["/control-plane-service"]
