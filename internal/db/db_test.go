package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOperationTimeoutTracerBoundsAcquire(t *testing.T) {
	tracer := &operationTimeoutTracer{timeout: 20 * time.Millisecond}
	ctx := tracer.TraceAcquireStart(context.Background(), nil, pgxpool.TraceAcquireStartData{})

	select {
	case <-ctx.Done():
		if ctx.Err() != context.DeadlineExceeded {
			t.Fatalf("acquire context error = %v, want deadline exceeded", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("acquire context did not reach its deadline")
	}
}

func TestOperationTimeoutTracerPreservesParentCancellation(t *testing.T) {
	tracer := &operationTimeoutTracer{timeout: time.Minute}
	parent, cancelParent := context.WithCancel(context.Background())
	ctx := tracer.TraceQueryStart(parent, nil, pgx.TraceQueryStartData{})

	cancelParent()
	select {
	case <-ctx.Done():
		if ctx.Err() != context.Canceled {
			t.Fatalf("query context error = %v, want canceled", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("query context did not inherit parent cancellation")
	}
}

func TestOperationTimeoutTracerCancelsCompletedOperations(t *testing.T) {
	tracer := &operationTimeoutTracer{timeout: time.Minute}

	acquireCtx := tracer.TraceAcquireStart(context.Background(), nil, pgxpool.TraceAcquireStartData{})
	tracer.TraceAcquireEnd(acquireCtx, nil, pgxpool.TraceAcquireEndData{})
	if err := acquireCtx.Err(); err != context.Canceled {
		t.Fatalf("completed acquire context error = %v, want canceled", err)
	}

	queryCtx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{})
	tracer.TraceQueryEnd(queryCtx, nil, pgx.TraceQueryEndData{})
	if err := queryCtx.Err(); err != context.Canceled {
		t.Fatalf("completed query context error = %v, want canceled", err)
	}
}
