FROM scratch

COPY bin/linux/vector-index-service /vector-index-service

ENTRYPOINT ["/vector-index-service"]
