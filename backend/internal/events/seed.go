package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/IkBenJur/italy-trip/internal/auth"
	"github.com/IkBenJur/italy-trip/internal/env"
	repo "github.com/IkBenJur/italy-trip/internal/postgres/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SeedUser is the one shared login. Registration is closed, so if this account
// does not exist there is no way into the app at all.
type SeedUser struct {
	Email    string
	Password string
}

func SeedUserFromEnv() SeedUser {
	return SeedUser{
		Email:    env.MustGet("SEED_USER_EMAIL"),
		Password: env.MustGet("SEED_USER_PASSWORD"),
	}
}

// Seed brings the database in line with the environment on every boot. It is
// idempotent: the event row is upserted and the shared user is only created if
// absent, so booting twice leaves exactly one of each.
//
// Syncing the event from env is what makes EVENT_ENDS_AT a Railway variable you
// can change with a redeploy rather than something needing manual SQL.
func Seed(ctx context.Context, queries repo.Querier, cfg Config, seedUser SeedUser) (Event, error) {
	row, err := queries.UpsertSingletonEvent(ctx, repo.UpsertSingletonEventParams{
		Name:     cfg.Name,
		StartsAt: pgtype.Timestamptz{Time: cfg.StartsAt, Valid: true},
		EndsAt:   pgtype.Timestamptz{Time: cfg.EndsAt, Valid: true},
	})
	if err != nil {
		return Event{}, fmt.Errorf("upsert event: %w", err)
	}

	event := FromRow(row)
	slog.Info("Event synced from env", "id", event.ID, "name", event.Name, "ends_at", event.EndsAt.UTC())

	if err := seedSharedUser(ctx, queries, seedUser); err != nil {
		return Event{}, err
	}

	return event, nil
}

func seedSharedUser(ctx context.Context, queries repo.Querier, seedUser SeedUser) error {
	existing, err := queries.FindUserByEmail(ctx, seedUser.Email)
	if err == nil {
		slog.Info("Shared user already present", "email", existing.Email)
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("look up shared user: %w", err)
	}

	hash, err := auth.HashPassword(seedUser.Password)
	if err != nil {
		return fmt.Errorf("hash shared user password: %w", err)
	}

	created, err := queries.CreateUser(ctx, repo.CreateUserParams{
		Email:        seedUser.Email,
		PasswordHash: hash,
	})
	if err != nil {
		return fmt.Errorf("create shared user: %w", err)
	}

	slog.Info("Shared user created", "email", created.Email)
	return nil
}
