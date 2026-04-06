package limiter

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

/*
KEYS[1] = sorted set key e.g. "user:123:default"
ARGV[1] = limit
ARGV[2] = window duration in seconds
ARGV[3] = current timestamp in milliseconds (for uniqueness)
*/
var slidingWindowScript = redis.NewScript(`
	local now = tonumber(ARGV[1])
	local limit = tonumber(ARGV[2])
	local windowSeconds = tonumber(ARGV[3])
	local window = windowSeconds * 1000
	local windowStart = now - window

	redis.call('ZREMRANGEBYSCORE', KEYS[1], 0, windowStart)

	local count = redis.call('ZCARD', KEYS[1])
	redis.call('EXPIRE', KEYS[1], ARGV[2])

	if count >= limit then
		local remaining = 0
		return {0, remaining}
	end

	redis.call('ZADD', KEYS[1], now, ARGV[4])
	local remaining = limit - count - 1
	if remaining < 0 then
		remaining = 0
	end

	return {1, remaining}
	`)

type SlidingWindowLua struct {
	client *redis.Client
}

func NewSlidingWindowLua(client *redis.Client) *SlidingWindowLua {
	return &SlidingWindowLua{client: client}
}

func (s *SlidingWindowLua) Allow(key string, policy string, limit int64, windowSeconds int64) (bool, int64, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	now := time.Now().UnixMilli()
	windowKey := fmt.Sprintf("%s:%s", key, policy)
	resetAt := (time.Now().Unix()/windowSeconds + 1) * windowSeconds
	uniqueID := fmt.Sprintf("%d-%d", now, rand.Int63())

	result, err := slidingWindowScript.Run(
		ctx,
		s.client,
		[]string{windowKey},
		now, limit, windowSeconds, uniqueID,
	).Int64Slice()

	if err != nil {
		return true, 0, resetAt, nil
	}

	allowed := result[0] == 1
	remaining := result[1]

	return allowed, remaining, resetAt, nil
}
