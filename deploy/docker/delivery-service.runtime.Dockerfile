FROM scratch

COPY bin/linux/delivery-service /delivery-service

ENTRYPOINT ["/delivery-service"]
