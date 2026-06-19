FROM scratch

COPY bin/linux/action-executor /action-executor

ENTRYPOINT ["/action-executor"]

