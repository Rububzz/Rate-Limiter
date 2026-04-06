package limiter

type Limiter interface {
	Allow(key string, policy string, limit int64, windowSeconds int64) (bool, int64, int64, error)
}

var _ Limiter = &FixedWindowRedis{}
var _ Limiter = &FixedWindowLua{}
var _ Limiter = &SlidingWindowLua{}
