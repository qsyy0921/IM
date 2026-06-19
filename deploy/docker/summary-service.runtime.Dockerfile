FROM scratch

COPY bin/linux/summary-service /summary-service

ENTRYPOINT ["/summary-service"]
