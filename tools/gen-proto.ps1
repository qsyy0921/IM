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
    "$ApiProtoDir/nexusim/delivery/v1/delivery_service.proto" `
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
