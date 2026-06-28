package main

import (
	"fmt"
	"math"
)

func buildUserPlan(cfg config) userPlan {
	owner := buildUser("hot-owner", 1, roleOwner, onlineFast)
	remaining := cfg.GroupSize - 1
	senders := make([]loadUser, 0, cfg.SenderCount)
	for index := 1; index <= cfg.SenderCount; index++ {
		senders = append(senders, buildUser("hot-sender", index, roleSender, onlineFast))
	}
	remaining -= len(senders)
	onlineCount := int(math.Round(float64(remaining) * cfg.OnlineRatio))
	if onlineCount > remaining {
		onlineCount = remaining
	}
	slowCount := int(math.Round(float64(onlineCount) * cfg.SlowClientRatio))
	if slowCount > onlineCount {
		slowCount = onlineCount
	}
	fastCount := onlineCount - slowCount
	offlineCount := remaining - onlineCount
	receivers := make([]loadUser, 0, remaining)
	for index := 1; index <= remaining; index++ {
		mode := offline
		switch {
		case index <= fastCount:
			mode = onlineFast
		case index <= fastCount+slowCount:
			mode = onlineSlow
		}
		receivers = append(receivers, buildUser("hot-user", index, roleReceiver, mode))
	}
	return userPlan{
		TenantID:       cfg.TenantID,
		ConversationID: cfg.ConversationID,
		GroupSize:      cfg.GroupSize,
		Owner:          owner,
		Senders:        senders,
		Receivers:      receivers,
		OnlineFast:     fastCount,
		OnlineSlow:     slowCount,
		Offline:        offlineCount,
	}
}

func buildUser(prefix string, index int, role userRole, mode onlineMode) loadUser {
	userID := fmt.Sprintf("%s-%06d", prefix, index)
	return loadUser{
		UserID:     userID,
		DeviceID:   fmt.Sprintf("%s-device-%06d", prefix, index),
		SessionID:  fmt.Sprintf("%s-session-%06d", prefix, index),
		Role:       role,
		OnlineMode: mode,
	}
}

func allMembers(plan userPlan) []loadUser {
	users := make([]loadUser, 0, plan.GroupSize)
	users = append(users, plan.Owner)
	users = append(users, plan.Senders...)
	users = append(users, plan.Receivers...)
	return users
}

func sampledReceivers(plan userPlan, limit int) []loadUser {
	if limit > len(plan.Receivers) {
		limit = len(plan.Receivers)
	}
	if limit <= 0 {
		return nil
	}
	return append([]loadUser(nil), plan.Receivers[:limit]...)
}
