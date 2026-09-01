package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/IkBenJur/italy-trip/internal/env"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upBackfillEventUserLinks, downBackfillEventUserLinks)
}

// upBackfillEventUserLinks points every pre-existing event at the seed user —
// the only account that could have created them before this column existed —
// and then locks user_id down with NOT NULL now that every row has a value.
// It also gives every user an active_event_id using the same "most recently
// created event" rule GetCurrentEvent uses.
//
// SEED_USER_EMAIL is read here rather than baked into the migration file so
// the same migration works across environments that seed a different address;
// SeedSharedUser (internal/events/seed.go) guarantees that user already
// exists by the time migrations run.
func upBackfillEventUserLinks(ctx context.Context, tx *sql.Tx) error {
	seedEmail := env.MustGet("SEED_USER_EMAIL")

	var seedUserID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE email = $1`, seedEmail).Scan(&seedUserID); err != nil {
		return fmt.Errorf("look up seed user %q: %w", seedEmail, err)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE events SET user_id = $1 WHERE user_id IS NULL`, seedUserID); err != nil {
		return fmt.Errorf("backfill events.user_id: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `ALTER TABLE events ALTER COLUMN user_id SET NOT NULL`); err != nil {
		return fmt.Errorf("enforce events.user_id NOT NULL: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET active_event_id = (SELECT id FROM events ORDER BY created_at DESC LIMIT 1)
		WHERE active_event_id IS NULL
	`); err != nil {
		return fmt.Errorf("backfill users.active_event_id: %w", err)
	}

	return nil
}

func downBackfillEventUserLinks(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `ALTER TABLE events ALTER COLUMN user_id DROP NOT NULL`); err != nil {
		return fmt.Errorf("relax events.user_id NOT NULL: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE events SET user_id = NULL`); err != nil {
		return fmt.Errorf("clear events.user_id: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET active_event_id = NULL`); err != nil {
		return fmt.Errorf("clear users.active_event_id: %w", err)
	}
	return nil
}
