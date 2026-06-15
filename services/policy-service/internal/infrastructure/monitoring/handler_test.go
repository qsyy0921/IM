package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestHandlerHealthz(t *testing.T) {
	handler := NewHandler(nil, false, nil, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body healthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if body.Service != serviceName || body.Status != "ok" {
		t.Fatalf("unexpected health response: %+v", body)
	}
}

func TestHandlerReadyzWithoutRequiredPool(t *testing.T) {
	handler := NewHandler(nil, false, nil, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body healthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode ready response: %v", err)
	}
	if body.Status != "ready" {
		t.Fatalf("unexpected ready response: %+v", body)
	}
}

func TestHandlerReadyzWithRequiredMissingPool(t *testing.T) {
	handler := NewHandler(nil, true, nil, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", response.Code)
	}
	var body healthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode ready response: %v", err)
	}
	if body.Status != "unready" || body.Error == "" {
		t.Fatalf("unexpected ready response: %+v", body)
	}
}

func TestHandlerMetricsIncludesGRPCAndDecisionSnapshots(t *testing.T) {
	grpcMetrics := NewGRPCMetrics()
	grpcMetrics.record("/nexusim.policy.v1.PolicyService/CheckMessageAction", "OK", 12)
	decisionMetrics := NewDecisionMetrics()
	decisionMetrics.Record("SEND", true, false, 7)
	decisionMetrics.Record("DELETE", false, false, 5)
	handler := NewHandler(nil, false, grpcMetrics, decisionMetrics)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	bodyRaw := response.Body.String()
	if strings.Contains(bodyRaw, "tenant-") ||
		strings.Contains(bodyRaw, "policy-message-user") ||
		strings.Contains(bodyRaw, "conversation") ||
		strings.Contains(bodyRaw, "message_id") {
		t.Fatalf("metrics should not expose high-cardinality identity fields: %s", bodyRaw)
	}
	var body Snapshot
	if err := json.Unmarshal([]byte(bodyRaw), &body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.GRPC == nil || body.GRPC.TotalRequests != 1 {
		t.Fatalf("expected grpc metrics, got %+v", body.GRPC)
	}
	if body.Decisions == nil || body.Decisions.Total != 2 || body.Decisions.Allowed != 1 || body.Decisions.Denied != 1 {
		t.Fatalf("expected decision metrics, got %+v", body.Decisions)
	}
}

func TestHandlerMetricsIncludesTraceSnapshot(t *testing.T) {
	handler := NewHandler(nil, false, nil, nil).WithTraceStats(func() TraceSnapshot {
		return TraceSnapshot{
			Enabled:         true,
			ServiceName:     serviceName,
			Exporter:        "stdout",
			SamplingRatio:   1,
			OTLPEndpointSet: false,
		}
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body Snapshot
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.Trace == nil || !body.Trace.Enabled || body.Trace.ServiceName != serviceName {
		t.Fatalf("expected trace snapshot, got %+v", body.Trace)
	}
}

func TestHandlerMetricsIncludesProjectionWorkerSnapshots(t *testing.T) {
	handler := NewHandler(nil, false, nil, nil).
		WithContactProjectionWorkerStats(func() types.ProjectionWorkerSnapshot {
			return types.ProjectionWorkerSnapshot{
				TotalErrors:        2,
				ConsecutiveErrors:  1,
				LastErrorAtMS:      100,
				LastSuccessAtMS:    90,
				LastCommitAtMS:     90,
				LastErrorBackoffMS: 1000,
			}
		}).
		WithTimelineProjectionWorkerStats(func() types.ProjectionWorkerSnapshot {
			return types.ProjectionWorkerSnapshot{
				TotalErrors:        3,
				ConsecutiveErrors:  0,
				LastErrorAtMS:      80,
				LastSuccessAtMS:    110,
				LastCommitAtMS:     110,
				LastErrorBackoffMS: 500,
			}
		})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body Snapshot
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.ContactProjectionWorker == nil || body.ContactProjectionWorker.TotalErrors != 2 {
		t.Fatalf("expected contact worker metrics, got %+v", body.ContactProjectionWorker)
	}
	if body.TimelineProjectionWorker == nil || body.TimelineProjectionWorker.TotalErrors != 3 {
		t.Fatalf("expected timeline worker metrics, got %+v", body.TimelineProjectionWorker)
	}
}

func TestHandlerMetricsIncludesOutboxRelaySnapshot(t *testing.T) {
	handler := NewHandler(nil, false, nil, nil).WithOutboxRelayStats(func() types.OutboxRelayWorkerSnapshot {
		return types.OutboxRelayWorkerSnapshot{
			TotalErrors:        2,
			ConsecutiveErrors:  1,
			LastErrorAtMS:      100,
			LastSuccessAtMS:    90,
			LastPublishedAtMS:  90,
			LastErrorBackoffMS: 1000,
		}
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body Snapshot
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.OutboxRelay == nil || body.OutboxRelay.TotalErrors != 2 {
		t.Fatalf("expected outbox relay snapshot, got %+v", body.OutboxRelay)
	}
}

func TestHandlerPrometheusMetricsWithoutPool(t *testing.T) {
	grpcMetrics := NewGRPCMetrics()
	grpcMetrics.record("/nexusim.policy.v1.PolicyService/CheckMessageAction", "OK", 12)
	grpcMetrics.record("/nexusim.policy.v1.PolicyService/CheckMessageAction", "PermissionDenied", 18)
	decisionMetrics := NewDecisionMetrics()
	decisionMetrics.Record("SEND", true, false, 7)
	decisionMetrics.Record("DELETE", false, false, 5)
	decisionMetrics.Record("EDIT", false, true, 9)
	handler := NewHandler(nil, false, grpcMetrics, decisionMetrics).
		WithTraceStats(func() TraceSnapshot {
			return TraceSnapshot{
				Enabled:         true,
				ServiceName:     serviceName,
				Exporter:        "otlp-grpc",
				OTLPEndpointSet: true,
				OTLPInsecure:    true,
				SamplingRatio:   0.25,
			}
		}).
		WithContactProjectionWorkerStats(func() types.ProjectionWorkerSnapshot {
			return types.ProjectionWorkerSnapshot{
				TotalErrors:        2,
				ConsecutiveErrors:  1,
				LastErrorAtMS:      100,
				LastSuccessAtMS:    90,
				LastCommitAtMS:     90,
				LastErrorBackoffMS: 1000,
			}
		}).
		WithTimelineProjectionWorkerStats(func() types.ProjectionWorkerSnapshot {
			return types.ProjectionWorkerSnapshot{
				TotalErrors:        3,
				ConsecutiveErrors:  0,
				LastErrorAtMS:      80,
				LastSuccessAtMS:    110,
				LastCommitAtMS:     110,
				LastErrorBackoffMS: 500,
			}
		}).
		WithOutboxRelayStats(func() types.OutboxRelayWorkerSnapshot {
			return types.OutboxRelayWorkerSnapshot{
				TotalErrors:        4,
				ConsecutiveErrors:  2,
				LastErrorAtMS:      120,
				LastSuccessAtMS:    130,
				LastPublishedAtMS:  140,
				LastErrorBackoffMS: 2000,
			}
		})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("expected prometheus content type, got %q", contentType)
	}
	body := response.Body.String()
	assertContains(t, body, `nexusim_policy_build_info{service="policy-service"} 1`)
	assertContains(t, body, `nexusim_policy_grpc_method_requests_total{method="/nexusim.policy.v1.PolicyService/CheckMessageAction"} 2`)
	assertContains(t, body, `nexusim_policy_grpc_requests_total{code="PermissionDenied",method="/nexusim.policy.v1.PolicyService/CheckMessageAction"} 1`)
	assertContains(t, body, `nexusim_policy_decisions_total{outcome="allowed"} 1`)
	assertContains(t, body, `nexusim_policy_decision_action_total{action="SEND",outcome="allowed"} 1`)
	assertContains(t, body, `nexusim_policy_projection_worker_errors_total{worker="contact"} 2`)
	assertContains(t, body, `nexusim_policy_projection_worker_errors_total{worker="timeline"} 3`)
	assertContains(t, body, `nexusim_policy_outbox_relay_errors_total 4`)
	assertContains(t, body, `nexusim_policy_otel_traces_enabled{exporter="otlp-grpc"} 1`)

	for _, forbidden := range []string{
		"tenant_id",
		"user_id",
		"device_id",
		"session_id",
		"request_id",
		"trace_id",
		"conversation_id",
		"message_id",
		"direct_peer",
		"sender_id",
		"payload",
		"classification",
		"deny_reason",
		"sql",
		"secret-token",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("prometheus metrics should not expose %q: %s", forbidden, body)
		}
	}
}

func TestRenderPrometheusIncludesPolicyAggregates(t *testing.T) {
	body := renderPrometheus(Snapshot{
		RuleStore: &RuleSnapshot{
			ExactMessageActions: &RuleDecisionSnapshot{
				Total: 3,
				Allow: 2,
				Deny:  1,
				Actions: []RuleActionSnapshot{
					{Action: "SEND", Total: 2, Allow: 2},
					{Action: "DELETE", Total: 1, Deny: 1},
				},
			},
			TenantMessageActions: &RuleDecisionSnapshot{
				Total: 2,
				Allow: 1,
				Deny:  1,
				Actions: []RuleActionSnapshot{
					{Action: "EDIT", Total: 2, Allow: 1, Deny: 1},
				},
			},
			ConversationRoleActions: &RuleRoleSnapshot{
				Total: 1,
				Actions: []RuleRoleActionSnapshot{
					{Action: "DELETE", MinRole: "ADMIN", Total: 1},
				},
			},
			OwnershipOverrides: &RuleRoleSnapshot{
				Total: 1,
				Actions: []RuleRoleActionSnapshot{
					{Action: "EDIT", MinRole: "OWNER", Total: 1},
				},
			},
		},
		Projection: &ProjectionSnapshot{
			ContactEdges: &ContactEdgeProjectionSnapshot{
				Total: 4, Active: 2, Blocked: 1, Deleted: 1,
			},
			ConversationMembers: &ConversationMemberProjectionSnapshot{
				Total: 5, Active: 3, Left: 1, Banned: 1,
				ByRole: []ProjectionGroupCountSnapshot{
					{Value: "OWNER", Total: 1},
				},
				ByStatus: []ProjectionGroupCountSnapshot{
					{Value: "ACTIVE", Total: 3},
				},
				ByPair: []ProjectionRoleStatusCount{
					{Role: "OWNER", Status: "ACTIVE", Total: 1},
				},
			},
			KafkaCheckpoints: &KafkaCheckpointSnapshot{
				Total: 2,
				Topics: []KafkaCheckpointTopicSnapshot{
					{
						Topic:          "im.contact.events",
						Rows:           2,
						ConsumerGroups: 1,
						Partitions:     2,
						MinOffsetValue: 11,
						MaxOffsetValue: 15,
					},
				},
			},
		},
		AuditOutbox: &AuditOutboxSnapshot{
			Total: 7, Pending: 2, Published: 4, DLQ: 1,
		},
	})

	assertContains(t, body, `nexusim_policy_rules{decision="total",scope="exact"} 3`)
	assertContains(t, body, `nexusim_policy_rule_actions{action="SEND",decision="allow",scope="exact"} 2`)
	assertContains(t, body, `nexusim_policy_role_rules{scope="conversation_role"} 1`)
	assertContains(t, body, `nexusim_policy_role_rule_actions{action="DELETE",min_role="ADMIN",scope="conversation_role"} 1`)
	assertContains(t, body, `nexusim_policy_contact_edges_projection{state="blocked"} 1`)
	assertContains(t, body, `nexusim_policy_conversation_members_projection{state="active"} 3`)
	assertContains(t, body, `nexusim_policy_conversation_members_by_role{role="OWNER"} 1`)
	assertContains(t, body, `nexusim_policy_conversation_members_by_status{status="ACTIVE"} 3`)
	assertContains(t, body, `nexusim_policy_conversation_members_by_role_status{role="OWNER",status="ACTIVE"} 1`)
	assertContains(t, body, `nexusim_policy_kafka_checkpoints{state="total"} 2`)
	assertContains(t, body, `nexusim_policy_kafka_checkpoints{state="rows",topic="im.contact.events"} 2`)
	assertContains(t, body, `nexusim_policy_kafka_checkpoint_topic_offsets{bound="max",topic="im.contact.events"} 15`)
	assertContains(t, body, `nexusim_policy_audit_outbox{state="dlq"} 1`)
}

func TestRenderPrometheusIncludesQueryErrorsAndEscapesLabels(t *testing.T) {
	body := renderPrometheus(Snapshot{
		RuleStoreError:           "policy rule metrics query failed",
		ProjectionError:          "policy projection metrics query failed",
		AuditOutboxError:         "policy audit outbox metrics query failed",
		RuleStore:                &RuleSnapshot{ExactMessageActions: &RuleDecisionSnapshot{Actions: []RuleActionSnapshot{{Action: "BAD\"ACT\\LINE\nNEXT", Total: 1}}}},
		Trace:                    &TraceSnapshot{Enabled: true, Exporter: "custom\"exporter", SamplingRatio: 1},
		GeneratedAtMS:            1,
		Service:                  serviceName,
		PGPool:                   nil,
		Projection:               nil,
		AuditOutbox:              nil,
		GRPC:                     nil,
		Decisions:                nil,
		OutboxRelay:              nil,
		ContactProjectionWorker:  nil,
		TimelineProjectionWorker: nil,
	})

	assertContains(t, body, `nexusim_policy_rule_store_query_error 1`)
	assertContains(t, body, `nexusim_policy_projection_query_error 1`)
	assertContains(t, body, `nexusim_policy_audit_outbox_query_error 1`)
	assertContains(t, body, `nexusim_policy_rule_actions{action="BAD\"ACT\\LINE\nNEXT",decision="total",scope="exact"} 1`)
	assertContains(t, body, `nexusim_policy_otel_traces_enabled{exporter="custom\"exporter"} 1`)
}

func TestQueryRuleSnapshotIncludesAllPolicyRuleStoresIntegration(t *testing.T) {
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	defer pool.Close()
	applyPolicyMonitoringMigrations(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
TRUNCATE
    policy_message_ownership_override_rules,
    policy_conversation_role_action_rules,
    policy_tenant_message_action_rules,
    policy_message_action_rules
`); err != nil {
		t.Fatalf("truncate policy rule tables: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO policy_message_action_rules (
    tenant_id, user_id, conversation_id, action, allowed, permission_version, classification
) VALUES
    ('tenant-metrics', 'user-1', 'conv-1', 'SEND', true, 1, 'EXACT_ALLOW'),
    ('tenant-metrics', 'user-2', 'conv-1', 'DELETE', false, 1, 'EXACT_DENY');
INSERT INTO policy_tenant_message_action_rules (
    tenant_id, action, allowed, permission_version, classification
) VALUES
    ('tenant-metrics', 'SEND', true, 1, 'TENANT_ALLOW'),
    ('tenant-metrics', 'EDIT', false, 1, 'TENANT_DENY');
INSERT INTO policy_conversation_role_action_rules (
    tenant_id, action, min_role, classification
) VALUES
    ('tenant-metrics', 'SEND', 'MEMBER', 'ROLE_MEMBER'),
    ('tenant-metrics', 'DELETE', 'ADMIN', 'ROLE_ADMIN');
INSERT INTO policy_message_ownership_override_rules (
    tenant_id, action, min_role, classification
) VALUES
    ('tenant-metrics', 'EDIT', 'ADMIN', 'OWNERSHIP_ADMIN'),
    ('tenant-metrics', 'DELETE', 'OWNER', 'OWNERSHIP_OWNER');
`); err != nil {
		t.Fatalf("seed policy rule tables: %v", err)
	}

	snapshot, err := queryRuleSnapshot(ctx, pool)
	if err != nil {
		t.Fatalf("query rule snapshot: %v", err)
	}
	if snapshot.Total != 2 || snapshot.Allow != 1 || snapshot.Deny != 1 || snapshot.ExactMessageActions == nil {
		t.Fatalf("unexpected exact rule snapshot: %+v", snapshot)
	}
	if snapshot.TenantMessageActions == nil ||
		snapshot.TenantMessageActions.Total != 2 ||
		snapshot.TenantMessageActions.Allow != 1 ||
		snapshot.TenantMessageActions.Deny != 1 {
		t.Fatalf("unexpected tenant rule snapshot: %+v", snapshot.TenantMessageActions)
	}
	if snapshot.ConversationRoleActions == nil || snapshot.ConversationRoleActions.Total != 2 || len(snapshot.ConversationRoleActions.Actions) != 2 {
		t.Fatalf("unexpected role rule snapshot: %+v", snapshot.ConversationRoleActions)
	}
	if snapshot.OwnershipOverrides == nil || snapshot.OwnershipOverrides.Total != 2 || len(snapshot.OwnershipOverrides.Actions) != 2 {
		t.Fatalf("unexpected ownership override snapshot: %+v", snapshot.OwnershipOverrides)
	}
}

func TestQueryProjectionSnapshotIncludesContactMemberAndCheckpointIntegration(t *testing.T) {
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	defer pool.Close()
	applyPolicyMonitoringMigrations(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
TRUNCATE
    policy_conversation_members_projection,
    policy_contact_edges_projection,
    policy_kafka_checkpoints
`); err != nil {
		t.Fatalf("truncate policy projection tables: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO policy_contact_edges_projection (
    tenant_id, owner_user_id, contact_user_id, status, edge_version, updated_by_event_id
) VALUES
    ('tenant-metrics', 'alice', 'bob', 'ACTIVE', 1, 'contact-event-1'),
    ('tenant-metrics', 'bob', 'alice', 'BLOCKED', 2, 'contact-event-2'),
    ('tenant-metrics', 'carol', 'dave', 'DELETED', 3, 'contact-event-3');
INSERT INTO policy_conversation_members_projection (
    tenant_id, conversation_id, user_id, role, status, member_version, permission_version, updated_by_event_id
) VALUES
    ('tenant-metrics', 'conv-1', 'alice', 'OWNER', 'ACTIVE', 1, 7, 'member-event-1'),
    ('tenant-metrics', 'conv-1', 'bob', 'ADMIN', 'ACTIVE', 2, 8, 'member-event-2'),
    ('tenant-metrics', 'conv-2', 'carol', 'MEMBER', 'LEFT', 3, 9, 'member-event-3'),
    ('tenant-metrics', 'conv-3', 'dave', 'MEMBER', 'BANNED', 4, 10, 'member-event-4');
INSERT INTO policy_kafka_checkpoints (
    consumer_group, topic, partition_id, offset_value
) VALUES
    ('policy-contact-group', 'im.contact.events', 0, 11),
    ('policy-contact-group', 'im.contact.events', 1, 15),
    ('policy-timeline-group', 'conversation.timeline.events', 0, 21);
`); err != nil {
		t.Fatalf("seed policy projection tables: %v", err)
	}

	snapshot, err := queryProjectionSnapshot(ctx, pool)
	if err != nil {
		t.Fatalf("query projection snapshot: %v", err)
	}
	if snapshot.ContactEdges == nil ||
		snapshot.ContactEdges.Total != 3 ||
		snapshot.ContactEdges.Active != 1 ||
		snapshot.ContactEdges.Blocked != 1 ||
		snapshot.ContactEdges.Deleted != 1 {
		t.Fatalf("unexpected contact projection snapshot: %+v", snapshot.ContactEdges)
	}
	if snapshot.ConversationMembers == nil ||
		snapshot.ConversationMembers.Total != 4 ||
		snapshot.ConversationMembers.Active != 2 ||
		snapshot.ConversationMembers.Left != 1 ||
		snapshot.ConversationMembers.Banned != 1 ||
		len(snapshot.ConversationMembers.ByRole) != 3 ||
		len(snapshot.ConversationMembers.ByStatus) != 3 ||
		len(snapshot.ConversationMembers.ByPair) != 4 {
		t.Fatalf("unexpected conversation member projection snapshot: %+v", snapshot.ConversationMembers)
	}
	if snapshot.KafkaCheckpoints == nil ||
		snapshot.KafkaCheckpoints.Total != 3 ||
		len(snapshot.KafkaCheckpoints.Topics) != 2 {
		t.Fatalf("unexpected checkpoint snapshot: %+v", snapshot.KafkaCheckpoints)
	}
	assertCheckpointTopic(t, snapshot.KafkaCheckpoints, "im.contact.events", 2, 1, 2, 11, 15)
	assertCheckpointTopic(t, snapshot.KafkaCheckpoints, "conversation.timeline.events", 1, 1, 1, 21, 21)
}

func applyPolicyMonitoringMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, name := range []string{
		"000001_policy_core.sql",
		"000002_policy_contacts_projection.sql",
		"000007_policy_tenant_message_action_rules.sql",
		"000008_policy_conversation_role_rules.sql",
		"000009_policy_message_ownership_override_rules.sql",
	} {
		path := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "policy", name)
		statement, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(statement)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
}

func assertCheckpointTopic(
	t *testing.T,
	snapshot *KafkaCheckpointSnapshot,
	topic string,
	rows int64,
	consumerGroups int64,
	partitions int64,
	minOffset int64,
	maxOffset int64,
) {
	t.Helper()
	for _, item := range snapshot.Topics {
		if item.Topic != topic {
			continue
		}
		if item.Rows != rows ||
			item.ConsumerGroups != consumerGroups ||
			item.Partitions != partitions ||
			item.MinOffsetValue != minOffset ||
			item.MaxOffsetValue != maxOffset {
			t.Fatalf("unexpected checkpoint topic %s: %+v", topic, item)
		}
		return
	}
	t.Fatalf("checkpoint topic %s not found in %+v", topic, snapshot.Topics)
}

func assertContains(t *testing.T, value string, expected string) {
	t.Helper()
	if !strings.Contains(value, expected) {
		t.Fatalf("expected to find %q in:\n%s", expected, value)
	}
}
