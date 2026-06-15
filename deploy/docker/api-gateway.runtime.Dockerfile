FROM scratch

COPY bin/linux/api-gateway /api-gateway

ENTRYPOINT ["/api-gateway"]
