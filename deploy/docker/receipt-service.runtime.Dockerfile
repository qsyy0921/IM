FROM scratch

COPY bin/linux/receipt-service /receipt-service

ENTRYPOINT ["/receipt-service"]
