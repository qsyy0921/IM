FROM scratch

COPY bin/linux/rag-service /rag-service

ENTRYPOINT ["/rag-service"]
