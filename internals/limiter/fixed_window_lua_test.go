package limiter

import (
	"testing"
	"time"
)

func TestFixedWindowLua_AllowsUnderLimit(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	fw := NewFixedWindowLua(client)

	allowed, remaining, _, err := fw.Allow("user:123", "default", 3, 60)

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

func TestFixedWindowLua_BlocksOverLimit(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	fw := NewFixedWindowLua(client)

	for i := 0; i < 3; i++ {
		fw.Allow("user:123", "default", 3, 60)
	}

	allowed, remaining, _, err := fw.Allow("user:123", "default", 3, 60)

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

func TestFixedWindowLua_DifferentUsersHaveIsolatedCounters(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	fw := NewFixedWindowLua(client)

	for i := 0; i < 3; i++ {
		fw.Allow("user:123", "default", 3, 60)
	}

	allowed, remaining, _, err := fw.Allow("user:456", "default", 3, 60)

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

func TestFixedWindowLua_DifferentPoliciesHaveIsolatedCounters(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	fw := NewFixedWindowLua(client)

	for i := 0; i < 3; i++ {
		fw.Allow("user:123", "default", 3, 60)
	}

	allowed, remaining, _, err := fw.Allow("user:123", "premium", 10, 60)

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

func TestFixedWindowLua_CounterResetsAfterWindow(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	fw := NewFixedWindowLua(client)

	for i := 0; i < 3; i++ {
		fw.Allow("user:123", "default", 3, 2)
	}

	time.Sleep(3 * time.Second)

	allowed, _, _, err := fw.Allow("user:123", "default", 3, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected request to be allowed after window reset")
	}
}
