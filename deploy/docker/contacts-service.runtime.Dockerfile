FROM scratch

COPY bin/linux/contacts-service /contacts-service

ENTRYPOINT ["/contacts-service"]
