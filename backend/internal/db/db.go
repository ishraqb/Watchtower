// Package db is the thin layer over TimescaleDB/Postgres. Ticks come in fast,
// so writes are batched with COPY rather than one INSERT per tick.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Tick is a single market data point.
type Tick struct {
	Time   time.Time
	Symbol string
	Price  float64
	Volume int
}

// DB wraps a pgx connection pool.
type DB struct {
	Pool *pgxpool.Pool
}

// New creates a connection pool and verifies connectivity with a ping.
func New(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: failed to create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping failed: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Close releases all pool connections.
func (d *DB) Close() {
	d.Pool.Close()
}

// BatchInsertTicks performs a high-throughput bulk insert using COPY.
// Prefer this over row-by-row INSERTs for high-frequency tick ingestion.
func (d *DB) BatchInsertTicks(ctx context.Context, ticks []Tick) (int64, error) {
	if len(ticks) == 0 {
		return 0, nil
	}

	rows := make([][]any, len(ticks))
	for i, t := range ticks {
		rows[i] = []any{t.Time, t.Symbol, t.Price, t.Volume}
	}

	n, err := d.Pool.CopyFrom(
		ctx,
		pgx.Identifier{"market_ticks"},
		[]string{"time", "symbol", "price", "volume"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return 0, fmt.Errorf("db: CopyFrom market_ticks failed: %w", err)
	}
	return n, nil
}

// InsertAnomaly records a detected anomaly and returns its generated id.
// Uses a parameterized query — never string interpolation — per SQL safety rules.
func (d *DB) InsertAnomaly(ctx context.Context, t time.Time, symbol, anomalyType string, triggerValue float64) (int, error) {
	var id int
	err := d.Pool.QueryRow(
		ctx,
		`INSERT INTO anomaly_events (time, symbol, anomaly_type, trigger_value)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		t, symbol, anomalyType, triggerValue,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("db: insert anomaly failed: %w", err)
	}
	return id, nil
}
