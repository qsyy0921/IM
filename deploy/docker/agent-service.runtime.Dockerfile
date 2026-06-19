FROM scratch

COPY bin/linux/agent-service /agent-service

ENTRYPOINT ["/agent-service"]
