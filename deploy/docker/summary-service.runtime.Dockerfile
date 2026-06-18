FROM gcr.io/distroless/static-debian12:nonroot
COPY bin/linux/summary-service /summary-service
USER nonroot:nonroot
ENTRYPOINT ["/summary-service"]
