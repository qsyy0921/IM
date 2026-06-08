// Package postgres implements message-service PostgreSQL repositories.
//
// It owns pgx/sqlc integration and maps database rows to domain objects. It
// must not decide message business semantics.
package postgres
