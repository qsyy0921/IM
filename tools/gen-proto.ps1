param(
    [string]$ApiProtoDir = "api/proto",
    [string]$KafkaSchemaDir = "schemas/kafka"
)

$ErrorActionPreference = "Stop"

protoc `
    -I $ApiProtoDir `
    --go_out=$ApiProtoDir `
    --go_opt=paths=source_relative `
    --go-grpc_out=$ApiProtoDir `
    --go-grpc_opt=paths=source_relative `
    "$ApiProtoDir/nexusim/actionexecutor/v1/action_executor_service.proto" `
    "$ApiProtoDir/nexusim/agent/v1/agent_service.proto" `
    "$ApiProtoDir/nexusim/contacts/v1/contacts_service.proto" `
    "$ApiProtoDir/nexusim/delivery/v1/delivery_service.proto" `
    "$ApiProtoDir/nexusim/gateway/v1/gateway_service.proto" `
    "$ApiProtoDir/nexusim/identity/v1/identity_service.proto" `
    "$ApiProtoDir/nexusim/memory/v1/memory_service.proto" `
    "$ApiProtoDir/nexusim/mcpgateway/v1/mcp_gateway_service.proto" `
    "$ApiProtoDir/nexusim/policy/v1/policy_service.proto" `
    "$ApiProtoDir/nexusim/rag/v1/rag_service.proto" `
    "$ApiProtoDir/nexusim/receipt/v1/receipt_service.proto" `
    "$ApiProtoDir/nexusim/retrieval/v1/retrieval_gateway.proto" `
    "$ApiProtoDir/nexusim/search/v1/search_service.proto" `
    "$ApiProtoDir/nexusim/skillregistry/v1/skill_registry_service.proto" `
    "$ApiProtoDir/nexusim/summary/v1/summary_service.proto" `
    "$ApiProtoDir/nexusim/conversation/v1/conversation_service.proto" `
    "$ApiProtoDir/nexusim/message/v1/message_error.proto" `
    "$ApiProtoDir/nexusim/message/v1/message_service.proto"

protoc `
    -I $KafkaSchemaDir `
    --go_out=$KafkaSchemaDir `
    --go_opt=paths=source_relative `
    "$KafkaSchemaDir/conversation.timeline.events.proto"

protoc `
    -I $KafkaSchemaDir `
    --go_out=$KafkaSchemaDir `
    --go_opt=paths=source_relative `
    "$KafkaSchemaDir/delivery/v1/im.delivery.events.proto"

protoc `
    -I $KafkaSchemaDir `
    --go_out=$KafkaSchemaDir `
    --go_opt=paths=source_relative `
    "$KafkaSchemaDir/receipt/v1/im.receipt.events.proto"

protoc `
    -I $KafkaSchemaDir `
    --go_out=$KafkaSchemaDir `
    --go_opt=paths=source_relative `
    "$KafkaSchemaDir/contacts/v1/im.contact.events.proto"

protoc `
    -I $KafkaSchemaDir `
    --go_out=$KafkaSchemaDir `
    --go_opt=paths=source_relative `
    "$KafkaSchemaDir/identity/v1/im.identity.events.proto"

protoc `
    -I $KafkaSchemaDir `
    --go_out=$KafkaSchemaDir `
    --go_opt=paths=source_relative `
    "$KafkaSchemaDir/policy/v1/im.policy.events.proto"
