FROM scratch

COPY bin/linux/model-gateway /model-gateway

ENTRYPOINT ["/model-gateway"]
