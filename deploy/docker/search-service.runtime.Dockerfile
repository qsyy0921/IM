FROM scratch

COPY bin/linux/search-service /search-service

ENTRYPOINT ["/search-service"]
