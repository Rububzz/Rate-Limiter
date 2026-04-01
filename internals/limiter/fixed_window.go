package limiter

import (
	"fmt"
	"sync"
	"time"
)

type FixedWindow struct {
	mu     sync.Mutex
	counts map[string]int64
}

func NewFixedWindow() *FixedWindow {
	return &FixedWindow{
		counts: make(map[string]int64),
	}
}

func (f *FixedWindow) Allow(key string, policy string, limit int64, windowSeconds int64) (bool, int64, int64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now().Unix()

	windowKey := fmt.Sprintf("%s:%s:%d", key, policy, now/windowSeconds)

	f.counts[windowKey]++
	count := f.counts[windowKey]

	resetAt := (now/windowSeconds + 1) * windowSeconds
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}
	allowed := remaining > 0
	return allowed, remaining, resetAt
}
