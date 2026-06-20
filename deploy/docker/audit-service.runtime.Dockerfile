FROM scratch

COPY bin/linux/audit-service /audit-service

ENTRYPOINT ["/audit-service"]
