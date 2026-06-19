FROM scratch

COPY bin/linux/mcp-gateway /mcp-gateway

ENTRYPOINT ["/mcp-gateway"]
