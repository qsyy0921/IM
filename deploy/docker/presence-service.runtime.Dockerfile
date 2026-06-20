FROM scratch

COPY bin/linux/presence-service /presence-service

ENTRYPOINT ["/presence-service"]
