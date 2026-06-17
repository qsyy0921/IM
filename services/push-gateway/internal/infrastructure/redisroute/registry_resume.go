package redisroute

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/qsyy0921/IM/services/push-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

func (registry *Registry) writeResumeMeta(ctx context.Context, token string, auth types.AuthContext) error {
	if token == "" {
		return nil
	}
	payload, err := json.Marshal(resumeMeta{
		TenantID: auth.TenantID,
		UserID:   auth.UserID,
		DeviceID: auth.DeviceID,
	})
	if err != nil {
		return err
	}
	pipe := registry.client.TxPipeline()
	pipe.Set(ctx, registry.resumeMetaKey(token), payload, registry.config.ResumeTTL)
	pipe.Expire(ctx, registry.resumeFramesKey(token), registry.config.ResumeTTL)
	_, err = pipe.Exec(ctx)
	return err
}

func (registry *Registry) loadRedisResume(ctx context.Context, token string) (redisResumeState, bool, error) {
	if token == "" {
		return redisResumeState{}, false, nil
	}
	rawMeta, err := registry.client.Get(ctx, registry.resumeMetaKey(token)).Result()
	if errors.Is(err, redis.Nil) {
		return redisResumeState{}, false, nil
	}
	if err != nil {
		return redisResumeState{}, false, err
	}
	var meta resumeMeta
	if err := json.Unmarshal([]byte(rawMeta), &meta); err != nil {
		return redisResumeState{}, false, nil
	}
	rawFrames, err := registry.client.LRange(ctx, registry.resumeFramesKey(token), 0, -1).Result()
	if err != nil {
		return redisResumeState{}, false, err
	}
	frames := make([]types.ServerFrame, 0, len(rawFrames))
	for _, rawFrame := range rawFrames {
		var frame types.ServerFrame
		if err := json.Unmarshal([]byte(rawFrame), &frame); err != nil {
			return redisResumeState{meta: meta}, true, nil
		}
		if isResumeFrame(frame) {
			frames = append(frames, frame)
		}
	}
	return redisResumeState{meta: meta, frames: frames}, true, nil
}

func (registry *Registry) appendRedisResume(ctx context.Context, token string, frame types.ServerFrame) error {
	if token == "" || !isResumeFrame(frame) {
		return nil
	}
	payload, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	pipe := registry.client.TxPipeline()
	pipe.RPush(ctx, registry.resumeFramesKey(token), payload)
	pipe.LTrim(ctx, registry.resumeFramesKey(token), int64(-types.DefaultResumeBufferSize), -1)
	pipe.Expire(ctx, registry.resumeMetaKey(token), registry.config.ResumeTTL)
	pipe.Expire(ctx, registry.resumeFramesKey(token), registry.config.ResumeTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	registry.metrics.resumeAppendCount.Add(1)
	return nil
}

func (registry *Registry) replayRedisResume(
	registration types.SessionRegistration,
	frames []types.ServerFrame,
) bool {
	if len(frames) == 0 {
		return false
	}
	lastReceived := make(map[string]int64, len(registration.LastReceived))
	for _, cursor := range registration.LastReceived {
		if cursor.ConversationID == "" {
			continue
		}
		if cursor.Seq > lastReceived[cursor.ConversationID] {
			lastReceived[cursor.ConversationID] = cursor.Seq
		}
	}
	oldestByConversation := make(map[string]int64)
	for _, frame := range frames {
		if frame.Op != types.OpDeliveryNotify || frame.ConversationID == "" {
			continue
		}
		if oldestByConversation[frame.ConversationID] == 0 ||
			frame.ConversationSeq < oldestByConversation[frame.ConversationID] {
			oldestByConversation[frame.ConversationID] = frame.ConversationSeq
		}
	}
	for conversationID, seq := range lastReceived {
		oldest := oldestByConversation[conversationID]
		if oldest > 0 && seq+1 < oldest {
			return true
		}
	}
	replayFrames := make([]types.ServerFrame, 0, len(frames))
	for _, frame := range frames {
		if !isResumeFrame(frame) {
			continue
		}
		if frame.Op == types.OpDeliveryNotify && frame.ConversationID != "" && frame.ConversationSeq <= lastReceived[frame.ConversationID] {
			continue
		}
		replayFrames = append(replayFrames, frame)
	}
	if len(replayFrames) == 0 {
		return false
	}
	if cap(registration.Outbound)-len(registration.Outbound) < len(replayFrames) {
		return true
	}
	for _, frame := range replayFrames {
		select {
		case registration.Outbound <- frame:
			registry.metrics.resumeReplayCount.Add(1)
		default:
			return true
		}
	}
	return false
}

func isResumeFrame(frame types.ServerFrame) bool {
	return frame.Op == types.OpDeliveryNotify || frame.Op == types.OpDeliveryHide
}

func (registry *Registry) enqueueResumeHint(outbound chan<- types.ServerFrame) {
	select {
	case outbound <- domain.ResumeHint("buffer_miss", nil):
	default:
	}
}

func sameDevice(left resumeMeta, right types.AuthContext) bool {
	return left.TenantID == right.TenantID &&
		left.UserID == right.UserID &&
		left.DeviceID == right.DeviceID
}

func (registry *Registry) resumeMetaKey(token string) string {
	return strings.Join([]string{registry.config.KeyPrefix, "resume", registry.resumeHashTag(token), "meta"}, ":")
}

func (registry *Registry) resumeFramesKey(token string) string {
	return strings.Join([]string{registry.config.KeyPrefix, "resume", registry.resumeHashTag(token), "frames"}, ":")
}

func (registry *Registry) resumeHashTag(token string) string {
	return "{resume:" + redisKeyPart(token) + "}"
}
