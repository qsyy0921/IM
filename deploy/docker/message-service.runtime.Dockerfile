FROM scratch

COPY bin/linux/message-service /message-service

ENTRYPOINT ["/message-service"]
