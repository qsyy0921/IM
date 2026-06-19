package postgres

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/skill-registry/internal/types"
)

func TestRepositorySkillLifecycleIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openSkillRegistryTestPool(t)
	resetSkillRegistryTables(t, ctx, pool)
	repository := NewRepository(pool)

	skill := types.SkillDefinition{
		TenantID:         "tenant-1",
		SkillID:          "skill-draft-reply",
		DisplayName:      "Draft Reply",
		Description:      "Draft a reply proposal from EvidencePack.",
		Version:          "v1",
		Status:           types.SkillStatusActive,
		ToolName:         "conversation.reply.draft",
		AllowedActions:   []int32{types.ToolActionCall},
		InputSchemaJSON:  `{"type":"object"}`,
		OutputSchemaJSON: `{"type":"object"}`,
		PermissionScope:  "conversation:write_proposal",
		RiskLevel:        "LOW",
		RequiresApproval: true,
		AuditEventType:   "agent.skill.proposed.v1",
		OwnerService:     "agent-service",
		Tags:             []string{"agent", "reply"},
		MetadataJSON:     `{"source":"test"}`,
	}
	created, err := repository.UpsertSkill(ctx, skill)
	if err != nil {
		t.Fatalf("upsert skill: %v", err)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps: %+v", created)
	}

	got, err := repository.GetSkill(ctx, "tenant-1", "skill-draft-reply")
	if err != nil {
		t.Fatalf("get skill: %v", err)
	}
	if got.ToolName != "conversation.reply.draft" || len(got.AllowedActions) != 1 || len(got.Tags) != 2 {
		t.Fatalf("unexpected skill: %+v", got)
	}

	updated := skill
	updated.Status = types.SkillStatusDisabled
	updated.Description = "updated"
	updated.AllowedActions = []int32{types.ToolActionCall, types.ToolActionApprove}
	if _, err := repository.UpsertSkill(ctx, updated); err != nil {
		t.Fatalf("update skill: %v", err)
	}

	active, err := repository.ListSkills(ctx, types.ListSkillsCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		Status:      types.SkillStatusActive,
	}, 10)
	if err != nil {
		t.Fatalf("list active skills: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected no active skills, got %+v", active)
	}

	disabled, err := repository.ListSkills(ctx, types.ListSkillsCommand{
		AuthContext:  types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		Status:       types.SkillStatusDisabled,
		OwnerService: "agent-service",
		ToolName:     "conversation.reply.draft",
		Tag:          "reply",
	}, 10)
	if err != nil {
		t.Fatalf("list disabled skills: %v", err)
	}
	if len(disabled) != 1 || disabled[0].Description != "updated" || len(disabled[0].AllowedActions) != 2 {
		t.Fatalf("unexpected disabled list: %+v", disabled)
	}

	if _, err := repository.GetSkill(ctx, "tenant-2", "skill-draft-reply"); err == nil {
		t.Fatal("expected tenant boundary to hide skill")
	}
}

func openSkillRegistryTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applySkillRegistryMigration(t, context.Background(), pool)
	return pool
}

func applySkillRegistryMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "skill-registry", "000001_skill_registry_core.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(content)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
}

func resetSkillRegistryTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `TRUNCATE skill_registry_definitions`); err != nil {
		t.Fatalf("reset skill registry tables: %v", err)
	}
}
