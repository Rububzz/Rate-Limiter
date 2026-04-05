package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type FixedWindowRedis struct {
	client *redis.Client
}

func NewFixedWindowRedis(client *redis.Client) *FixedWindowRedis {
	return &FixedWindowRedis{client: client}
}

func (f *FixedWindowRedis) Allow(key string, limit int64, windowSeconds int64) (bool, int64, int64, error) {
	ctx := context.Background()
	now := time.Now().Unix()

	windowKey := fmt.Sprintf("%s:%s:%d", key, policy, now/windowSeconds)

	pipe := f.client.Pipeline()
	incr := pipe.Incr(ctx, windowKey)
	pipe.Expire(ctx, windowKey, time.Duration(windowSeconds)*time.Second)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return true, 0, 0, nil
	}

	count := incr.Val()
	remaining := limit - count
	reset := now + (windowSeconds - (now % windowSeconds))

	allowed := remaining > 0
	return allowed, remaining, reset, nil

}
