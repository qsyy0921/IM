FROM scratch

COPY bin/linux/media-service /media-service

ENTRYPOINT ["/media-service"]
