FROM scratch

COPY bin/linux/ai-eval-service /ai-eval-service

ENTRYPOINT ["/ai-eval-service"]
