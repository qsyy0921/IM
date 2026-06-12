FROM scratch

COPY bin/linux/identity-service /identity-service

ENTRYPOINT ["/identity-service"]
