FROM scratch

COPY bin/linux/notification-service /notification-service

ENTRYPOINT ["/notification-service"]
