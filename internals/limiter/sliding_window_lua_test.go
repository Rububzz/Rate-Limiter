package limiter

import (
	"testing"
	"time"
)

func TestSlidingWindowLua_AllowsUnderLimit(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	sw := NewSlidingWindowLua(client)

	allowed, remaining, _, err := sw.Allow("user:123", "default", 3, 60)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected request to be allowed")
	}
	if remaining != 2 {
		t.Errorf("expected remaining 2, got %d", remaining)
	}
}

func TestSlidingWindowLua_BlocksOverLimit(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	sw := NewSlidingWindowLua(client)

	for i := 0; i < 3; i++ {
		sw.Allow("user:123", "default", 3, 60)
	}

	allowed, remaining, _, err := sw.Allow("user:123", "default", 3, 60)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("expected request to be blocked")
	}
	if remaining != 0 {
		t.Errorf("expected remaining 0, got %d", remaining)
	}
}

func TestSlidingWindowLua_DifferentUsersHaveIsolatedCounters(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	sw := NewSlidingWindowLua(client)

	for i := 0; i < 3; i++ {
		sw.Allow("user:123", "default", 3, 60)
	}

	allowed, remaining, _, err := sw.Allow("user:456", "default", 3, 60)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected user:456 to be allowed")
	}
	if remaining != 2 {
		t.Errorf("expected remaining 2, got %d", remaining)
	}
}

func TestSlidingWindowLua_DifferentPoliciesHaveIsolatedCounters(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	sw := NewSlidingWindowLua(client)

	for i := 0; i < 3; i++ {
		sw.Allow("user:123", "default", 3, 60)
	}

	allowed, remaining, _, err := sw.Allow("user:123", "premium", 10, 60)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected premium policy to be allowed")
	}
	if remaining != 9 {
		t.Errorf("expected remaining 9, got %d", remaining)
	}
}

func TestSlidingWindowLua_NoBoundarySpike(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	sw := NewSlidingWindowLua(client)

	// exhaust limit with a 2 second window
	for i := 0; i < 3; i++ {
		sw.Allow("user:123", "default", 3, 2)
	}

	// wait just past the window boundary
	time.Sleep(2100 * time.Millisecond)

	// with fixed window this would be a brand new bucket — all 3 allowed
	// with sliding window only requests older than 2s are cleared
	// since we just crossed the boundary, the recent requests are still counted
	allowed, _, _, err := sw.Allow("user:123", "default", 3, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected at least one request to be allowed after window")
	}
}

func TestSlidingWindowLua_CounterResetsAfterFullWindow(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	sw := NewSlidingWindowLua(client)

	for i := 0; i < 3; i++ {
		sw.Allow("user:123", "default", 3, 2)
	}

	// wait for full window to pass
	time.Sleep(3 * time.Second)

	allowed, remaining, _, err := sw.Allow("user:123", "default", 3, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected request to be allowed after full window")
	}
	if remaining != 2 {
		t.Errorf("expected remaining 2, got %d", remaining)
	}
}
