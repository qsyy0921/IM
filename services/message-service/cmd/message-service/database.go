package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func openPGPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns := envInt("NEXUSIM_PG_MAX_CONNS", 0); maxConns > 0 {
		config.MaxConns = int32(maxConns)
	}
	if minConns := envInt("NEXUSIM_PG_MIN_CONNS", 0); minConns > 0 {
		config.MinConns = int32(minConns)
	}
	return pgxpool.NewWithConfig(ctx, config)
}
