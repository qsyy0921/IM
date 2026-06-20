package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/qsyy0921/IM/services/control-plane-service/internal/types"
)

type PreparedConfigVersion struct {
	Command         types.PublishConfigVersionCommand
	PayloadJSON     string
	PayloadChecksum string
	CommandHash     string
}

func PrepareConfigVersion(command types.PublishConfigVersionCommand) (PreparedConfigVersion, error) {
	if err := command.Validate(); err != nil {
		return PreparedConfigVersion{}, err
	}
	normalized := command.Normalized()
	canonicalPayload, err := CanonicalJSON(normalized.PayloadJSON)
	if err != nil {
		return PreparedConfigVersion{}, err
	}
	normalized.PayloadJSON = canonicalPayload
	checksum := SHA256String(canonicalPayload)
	commandHash, err := CanonicalCommandHash(normalized)
	if err != nil {
		return PreparedConfigVersion{}, err
	}
	return PreparedConfigVersion{
		Command:         normalized,
		PayloadJSON:     canonicalPayload,
		PayloadChecksum: checksum,
		CommandHash:     commandHash,
	}, nil
}

func CanonicalJSON(value string) (string, error) {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &decoded); err != nil {
		return "", types.NewInvalidArgument("payload_json must be valid json")
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return "", types.NewInvalidArgument("payload_json must be canonicalizable")
	}
	return string(encoded), nil
}

func CanonicalCommandHash(command types.PublishConfigVersionCommand) (string, error) {
	payload := map[string]any{
		"tenant_id":       string(command.AuthContext.TenantID),
		"environment":     command.Environment,
		"config_kind":     command.ConfigKind,
		"bundle_key":      command.BundleKey,
		"version":         command.Version,
		"schema_version":  command.SchemaVersion,
		"payload_json":    command.PayloadJSON,
		"effective_at":    command.EffectiveAt.UnixMilli(),
		"expires_at":      command.ExpiresAt.UnixMilli(),
		"approval_ref":    command.ApprovalRef,
		"operator_ref":    command.OperatorRef,
		"reason_ref":      command.ReasonRef,
		"idempotency_key": command.IdempotencyKey,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewInvalidArgument("command hash payload invalid")
	}
	return SHA256String(string(encoded)), nil
}

func SHA256String(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
