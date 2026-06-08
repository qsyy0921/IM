FROM scratch

COPY bin/linux/sendmessage-loadtest /sendmessage-loadtest

ENTRYPOINT ["/sendmessage-loadtest"]
