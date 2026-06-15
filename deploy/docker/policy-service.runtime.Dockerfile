FROM scratch

COPY bin/linux/policy-service /policy-service

ENTRYPOINT ["/policy-service"]
