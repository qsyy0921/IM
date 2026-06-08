PROTO_API_DIR := api/proto
PROTO_KAFKA_DIR := schemas/kafka

.PHONY: proto
proto:
	protoc \
		-I $(PROTO_API_DIR) \
		--go_out=$(PROTO_API_DIR) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(PROTO_API_DIR) \
		--go-grpc_opt=paths=source_relative \
		$(PROTO_API_DIR)/nexusim/message/v1/message_error.proto \
		$(PROTO_API_DIR)/nexusim/message/v1/message_service.proto
	protoc \
		-I $(PROTO_KAFKA_DIR) \
		--go_out=$(PROTO_KAFKA_DIR) \
		--go_opt=paths=source_relative \
		$(PROTO_KAFKA_DIR)/conversation.timeline.events.proto

.PHONY: local-up
local-up:
	docker compose -f deploy/local/docker-compose.yml up -d

.PHONY: local-down
local-down:
	docker compose -f deploy/local/docker-compose.yml down

.PHONY: local-logs
local-logs:
	docker compose -f deploy/local/docker-compose.yml logs -f
