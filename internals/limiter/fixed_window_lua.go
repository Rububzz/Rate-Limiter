package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var fixedWindowScript = redis.NewScript(`
	local count = redis.call('INCR', KEYS[1])
	if count == 1 then
		redis.call('EXPIRE', KEYS[1], ARGV[2])
	end

	local remaining = tonumber(ARGV[1]) - count
	if remaining < 0 then
		remaining = 0
	end

	local allowed = 1
	if count > tonumber(ARGV[1]) then 
		allowed = 0
	end
	return {count, remaining, allowed}
	`)

type FixedWindowLua struct {
	client *redis.Client
}

func NewFixedWindowLua(client *redis.Client) *FixedWindowLua {
	return &FixedWindowLua{client: client}
}

func (f *FixedWindowLua) Allow(key string, policy string, limit int64, windowSeconds int64) (bool, int64, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	now := time.Now().Unix()
	windowKey := fmt.Sprintf("%s:%s:%d", key, policy, now/windowSeconds)
	resetAt := (now/windowSeconds + 1) * windowSeconds
	result, err := fixedWindowScript.Run(
		ctx,
		f.client,
		[]string{windowKey},
		limit, windowSeconds,
	).Int64Slice()

	if err != nil {
		// fail open — if Redis is down, allow the request
		return true, 0, resetAt, nil
	}

	remaining := result[1]
	allowed := result[2] == 1

	return allowed, remaining, resetAt, nil

}
