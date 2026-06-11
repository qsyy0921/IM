FROM scratch

COPY bin/linux/conversation-service /conversation-service

ENTRYPOINT ["/conversation-service"]
