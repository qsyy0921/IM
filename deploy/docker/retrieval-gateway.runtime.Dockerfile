FROM scratch

COPY bin/linux/retrieval-gateway /retrieval-gateway

ENTRYPOINT ["/retrieval-gateway"]
