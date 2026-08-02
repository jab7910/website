package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultConnectTimeout    = 10 * time.Second
	databaseOperationTimeout = 15 * time.Second
)

type operationTimeoutCancelKey struct{}

// operationTimeoutTracer bounds both phases of a pooled query. Acquisition
// happens before PostgreSQL can apply statement_timeout, so it needs its own
// client-side deadline.
type operationTimeoutTracer struct {
	timeout time.Duration
}

var (
	_ pgx.QueryTracer       = (*operationTimeoutTracer)(nil)
	_ pgxpool.AcquireTracer = (*operationTimeoutTracer)(nil)
)

func (t *operationTimeoutTracer) withTimeout(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	timedCtx, cancel := context.WithTimeout(ctx, t.timeout)
	return context.WithValue(timedCtx, operationTimeoutCancelKey{}, cancel)
}

func cancelOperationTimeout(ctx context.Context) {
	if cancel, ok := ctx.Value(operationTimeoutCancelKey{}).(context.CancelFunc); ok {
		cancel()
	}
}

func (t *operationTimeoutTracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	return t.withTimeout(ctx)
}

func (t *operationTimeoutTracer) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireEndData) {
	cancelOperationTimeout(ctx)
}

func (t *operationTimeoutTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return t.withTimeout(ctx)
}

func (t *operationTimeoutTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	cancelOperationTimeout(ctx)
}

func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.HealthCheckPeriod = time.Minute
	cfg.ConnConfig.Tracer = &operationTimeoutTracer{timeout: databaseOperationTimeout}
	// Keep a blocked statement or forgotten transaction from permanently
	// removing a connection from the small application pool. The timeout tracer
	// also bounds pool acquisition, which server-side settings cannot.
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "15000"
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "5000"
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "30000"
	cfg.ConnConfig.RuntimeParams["application_name"] = "btcpp-web"

	connectCtx, cancel := context.WithTimeout(ctx, defaultConnectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connectCtx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}
