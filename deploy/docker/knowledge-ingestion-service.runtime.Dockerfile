FROM scratch

COPY bin/linux/knowledge-ingestion-service /knowledge-ingestion-service

ENTRYPOINT ["/knowledge-ingestion-service"]
