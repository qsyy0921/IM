FROM scratch

COPY bin/linux/workflow-service /workflow-service

ENTRYPOINT ["/workflow-service"]
