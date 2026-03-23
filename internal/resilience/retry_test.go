package resilience

import (
	"context"
	"errors"
	"testing"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

func TestDo_Success(t *testing.T) {
	calls := 0
	err := Do(context.Background(), DefaultRetryConfig(), func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestDo_RetryOnTransient(t *testing.T) {
	calls := 0
	err := Do(context.Background(), DefaultRetryConfig(), func() error {
		calls++
		if calls < 3 {
			return &clients.GatewayError{StatusCode: 503, Code: "upstream_unavailable", Message: "down"}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestDo_NoRetryOn4xx(t *testing.T) {
	calls := 0
	err := Do(context.Background(), DefaultRetryConfig(), func() error {
		calls++
		return &clients.GatewayError{StatusCode: 404, Code: "not_found", Message: "gone"}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on 4xx)", calls)
	}
}

func TestDo_MaxRetriesExhausted(t *testing.T) {
	calls := 0
	err := Do(context.Background(), DefaultRetryConfig(), func() error {
		calls++
		return &clients.GatewayError{StatusCode: 503, Code: "upstream_unavailable", Message: "down"}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// 1 initial + 3 retries = 4 total
	if calls != 4 {
		t.Errorf("calls = %d, want 4", calls)
	}
}

func TestDo_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	err := Do(ctx, DefaultRetryConfig(), func() error {
		calls++
		return &clients.GatewayError{StatusCode: 503, Code: "upstream_unavailable", Message: "down"}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// Should stop retrying quickly after context is cancelled
	if calls > 2 {
		t.Errorf("calls = %d, expected <= 2 with cancelled context", calls)
	}
}

func TestDo_NonRetryableError(t *testing.T) {
	calls := 0
	err := Do(context.Background(), DefaultRetryConfig(), func() error {
		calls++
		return errors.New("some other error")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on non-retryable)", calls)
	}
}
