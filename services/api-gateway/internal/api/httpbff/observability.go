package httpbff

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type MetricsRecorder interface {
	RecordHTTPBFF(route string, method string, statusCode int, latency time.Duration)
}

type RateLimiter interface {
	Check(context.Context, string) error
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func newStatusRecorder(response http.ResponseWriter) *statusRecorder {
	return &statusRecorder{ResponseWriter: response, code: http.StatusOK}
}

func (recorder *statusRecorder) WriteHeader(code int) {
	recorder.code = code
	recorder.ResponseWriter.WriteHeader(code)
}

func (recorder *statusRecorder) statusCode() int {
	if recorder.code == 0 {
		return http.StatusOK
	}
	return recorder.code
}

func (server *Server) recordMetrics(route string, method string, statusCode int, latency time.Duration) {
	if server == nil || server.metrics == nil {
		return
	}
	server.metrics.RecordHTTPBFF(route, method, statusCode, latency)
}

func (server *Server) checkRateLimit(response http.ResponseWriter, request *http.Request, route string) bool {
	if server == nil || server.rateLimiter == nil {
		return true
	}
	if route == "health" || route == "cors.preflight" {
		return true
	}
	if err := server.rateLimiter.Check(contextFromRequest(request), rateLimitMethod(route)); err != nil {
		writeError(response, err)
		return false
	}
	return true
}

func rateLimitMethod(route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		route = "unknown"
	}
	return "/nexusim.api_gateway.bff.HTTPBFF/" + route
}

func RouteName(request *http.Request) string {
	if request == nil || request.URL == nil {
		return "unknown"
	}
	if request.Method == http.MethodOptions {
		return "cors.preflight"
	}
	path := strings.TrimRight(request.URL.Path, "/")
	if path == "" {
		path = "/"
	}
	switch {
	case request.Method == http.MethodGet && path == "/api/health":
		return "health"
	case request.Method == http.MethodPost && path == "/api/auth/login":
		return "auth.login"
	case request.Method == http.MethodPost && path == "/api/auth/register":
		return "auth.register"
	case request.Method == http.MethodPost && path == "/api/auth/refresh":
		return "auth.refresh"
	case request.Method == http.MethodPost && path == "/api/auth/logout":
		return "auth.logout"
	case request.Method == http.MethodGet && path == "/api/me":
		return "me"
	case request.Method == http.MethodGet && path == "/api/conversations":
		return "conversations.list"
	case request.Method == http.MethodPost && path == "/api/conversations/create":
		return "conversations.create"
	case request.Method == http.MethodPost && path == "/api/conversations/direct":
		return "conversations.direct"
	case request.Method == http.MethodGet && isConversationMemberActionPath(request.URL.EscapedPath(), "/members"):
		return "conversations.members.list"
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/members/invite"):
		return "conversations.members.invite"
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/members/leave"):
		return "conversations.members.leave"
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/members/remove"):
		return "conversations.members.remove"
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/members/role"):
		return "conversations.members.role"
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/owner/transfer"):
		return "conversations.owner.transfer"
	case request.Method == http.MethodGet && isConversationMessagesPath(request.URL.EscapedPath()):
		return "conversation.messages"
	case request.Method == http.MethodPost && path == "/api/messages/send":
		return "messages.send"
	case request.Method == http.MethodPost && path == "/api/delivery/ack":
		return "delivery.ack"
	case request.Method == http.MethodGet && path == "/api/contact-requests":
		return "contact_requests.list"
	case request.Method == http.MethodPost && path == "/api/contact-requests/send":
		return "contact_requests.send"
	case request.Method == http.MethodPost && path == "/api/contact-requests/respond":
		return "contact_requests.respond"
	case request.Method == http.MethodPost && path == "/api/contact-requests/cancel":
		return "contact_requests.cancel"
	case request.Method == http.MethodGet && path == "/api/contacts":
		return "contacts.list"
	case request.Method == http.MethodGet && path == "/api/contacts/state":
		return "contacts.state"
	case request.Method == http.MethodPost && path == "/api/contacts/delete":
		return "contacts.delete"
	case request.Method == http.MethodPost && path == "/api/contacts/block":
		return "contacts.block"
	case request.Method == http.MethodPost && path == "/api/contacts/unblock":
		return "contacts.unblock"
	case request.Method == http.MethodPost && path == "/api/contacts/remark":
		return "contacts.remark"
	case request.Method == http.MethodPost && path == "/api/contacts/group":
		return "contacts.group"
	case request.Method == http.MethodGet && path == "/api/receipts":
		return "receipts.list"
	default:
		return "not_found"
	}
}
