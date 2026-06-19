FROM gcr.io/distroless/static-debian12:nonroot
COPY bin/linux/agent-service /agent-service
USER nonroot:nonroot
ENTRYPOINT ["/agent-service"]
