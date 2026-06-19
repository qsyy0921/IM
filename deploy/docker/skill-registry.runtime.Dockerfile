FROM scratch

COPY bin/linux/skill-registry /skill-registry

ENTRYPOINT ["/skill-registry"]
