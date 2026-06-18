FROM scratch

COPY bin/linux/memory-service /memory-service

ENTRYPOINT ["/memory-service"]
