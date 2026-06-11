FROM scratch

COPY bin/linux/push-gateway /push-gateway

ENTRYPOINT ["/push-gateway"]
