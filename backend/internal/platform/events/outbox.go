package events

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Execer is satisfied by pgx.Tx (and *pgxpool.Pool, though enqueueing
// outside a transaction defeats the purpose of the outbox).
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Enqueue stores ev in the outbox using tx, so it commits atomically with the
// caller's state change. Pass the same transaction that writes the state.
func Enqueue(ctx context.Context, tx Execer, ev Event) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO outbox (id, type, source, occurred_at, payload) VALUES ($1, $2, $3, $4, $5)`,
		ev.ID, ev.Type, ev.Source, ev.OccurredAt, ev.Data)
	return err
}

// Publisher sends one event to the broker.
type Publisher interface {
	Publish(ctx context.Context, ev Event) error
}

// outboxStore abstracts the outbox table so the relay logic is unit-testable.
// ClaimBatch locks up to limit pending events (oldest first), calls publish
// with them, and marks the ids publish returns as published, all in one
// database transaction.
type outboxStore interface {
	ClaimBatch(ctx context.Context, limit int, publish func([]Event) []string) error
}

// Relay moves events from the outbox to the broker.
type Relay struct {
	store    outboxStore
	pub      Publisher
	batch    int
	interval time.Duration
}

// NewRelay returns a relay polling pool's outbox every interval.
func NewRelay(pool *pgxpool.Pool, pub Publisher, interval time.Duration) *Relay {
	return &Relay{store: pgOutbox{pool: pool}, pub: pub, batch: 100, interval: interval}
}

// Run flushes the outbox until ctx is cancelled. Multiple replicas may run a
// relay concurrently: rows are claimed with FOR UPDATE SKIP LOCKED.
func (r *Relay) Run(ctx context.Context) {
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		if n, err := r.Flush(ctx); err != nil {
			slog.Warn("outbox flush failed", "err", err)
		} else if n > 0 {
			slog.Debug("outbox flushed", "published", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Flush publishes one batch of pending events in order and returns how many
// were published. Publishing stops at the first failure so per-producer
// ordering is preserved; the rest are retried on the next tick.
func (r *Relay) Flush(ctx context.Context) (int, error) {
	published := 0
	err := r.store.ClaimBatch(ctx, r.batch, func(evs []Event) []string {
		ids := make([]string, 0, len(evs))
		for _, ev := range evs {
			if err := r.pub.Publish(ctx, ev); err != nil {
				slog.Warn("event publish failed; will retry", "type", ev.Type, "id", ev.ID, "err", err)
				break
			}
			ids = append(ids, ev.ID)
		}
		published = len(ids)
		return ids
	})
	return published, err
}

// pgOutbox is the Postgres implementation of outboxStore.
type pgOutbox struct{ pool *pgxpool.Pool }

func (p pgOutbox) ClaimBatch(ctx context.Context, limit int, publish func([]Event) []string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id::text, type, source, occurred_at, payload FROM outbox
		WHERE published_at IS NULL ORDER BY occurred_at
		LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return err
	}
	evs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Event, error) {
		var e Event
		err := row.Scan(&e.ID, &e.Type, &e.Source, &e.OccurredAt, &e.Data)
		return e, err
	})
	if err != nil || len(evs) == 0 {
		return err
	}
	ids := publish(evs)
	if len(ids) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id = ANY($1::uuid[])`, ids); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
