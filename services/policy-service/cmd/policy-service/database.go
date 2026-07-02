package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func openPGPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	return openPGPoolWithMaxConns(ctx, dsn, envInt("NEXUSIM_POLICY_PG_MAX_CONNS", 0))
}

func openPolicyAuditPGPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	return openPGPoolWithMaxConns(ctx, dsn, envInt("NEXUSIM_POLICY_AUDIT_PG_MAX_CONNS", 32))
}

func openPGPoolWithMaxConns(ctx context.Context, dsn string, maxConns int) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns > 0 {
		config.MaxConns = int32(maxConns)
	}
	return pgxpool.NewWithConfig(ctx, config)
}
