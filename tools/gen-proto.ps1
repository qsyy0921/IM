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
    "$ApiProtoDir/nexusim/message/v1/message_error.proto" `
    "$ApiProtoDir/nexusim/message/v1/message_service.proto"

protoc `
    -I $KafkaSchemaDir `
    --go_out=$KafkaSchemaDir `
    --go_opt=paths=source_relative `
    "$KafkaSchemaDir/conversation.timeline.events.proto"
