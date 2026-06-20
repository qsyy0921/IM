FROM scratch

COPY bin/linux/admin-service /admin-service

ENTRYPOINT ["/admin-service"]
