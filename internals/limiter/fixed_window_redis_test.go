package limiter

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// helper to create a fresh Redis client for each test
func newTestRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
}

// helper to clean up keys after each test so they don't interfere
func flushRedis(t *testing.T, client *redis.Client) {
	t.Helper()
	client.FlushAll(t.Context())
}

func TestFixedWindowRedis_AllowsUnderLimit(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	fw := NewFixedWindowRedis(client)

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

func TestFixedWindowRedis_BlocksOverLimit(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	fw := NewFixedWindowRedis(client)

	// exhaust the limit
	for i := 0; i < 3; i++ {
		fw.Allow("user:123", "default", 3, 60)
	}

	// 4th request should be blocked
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

func TestFixedWindowRedis_DifferentUsersHaveIsolatedCounters(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	fw := NewFixedWindowRedis(client)

	// exhaust user:123
	for i := 0; i < 3; i++ {
		fw.Allow("user:123", "default", 3, 60)
	}

	// user:456 should be unaffected
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

func TestFixedWindowRedis_DifferentPoliciesHaveIsolatedCounters(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	fw := NewFixedWindowRedis(client)

	// exhaust default policy for user:123
	for i := 0; i < 3; i++ {
		fw.Allow("user:123", "default", 3, 60)
	}

	// premium policy should have its own counter
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

func TestFixedWindowRedis_CounterResetsAfterWindow(t *testing.T) {
	client := newTestRedisClient()
	defer flushRedis(t, client)

	fw := NewFixedWindowRedis(client)

	// exhaust limit with a 2 second window
	for i := 0; i < 3; i++ {
		fw.Allow("user:123", "default", 3, 2)
	}

	blocked, _, _, _ := fw.Allow("user:123", "default", 3, 2)
	if blocked {
		t.Error("expected 4th request to be blocked before window resets")
	}

	// wait for window to expire
	time.Sleep(3 * time.Second)

	// should be allowed again
	allowed, _, _, err := fw.Allow("user:123", "default", 3, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected request to be allowed after window reset")
	}
}
