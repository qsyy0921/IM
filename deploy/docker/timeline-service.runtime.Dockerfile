FROM scratch

COPY bin/linux/timeline-service /timeline-service

ENTRYPOINT ["/timeline-service"]
