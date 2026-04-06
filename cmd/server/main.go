package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Rububzz/Rate-Limiter/internals/limiter"
	"github.com/redis/go-redis/v9"
)

type LimitConfig struct {
	Limit         int64
	WindowSeconds int64
}

var policies = map[string]LimitConfig{
	"default":       {Limit: 3, WindowSeconds: 60},
	"premium":       {Limit: 10, WindowSeconds: 60},
	"auth_endpoint": {Limit: 5, WindowSeconds: 60},
}

type CheckRequest struct {
	Key    string `json:"key"`
	Policy string `json:"policy"`
}

type CheckResponse struct {
	Allowed   bool   `json:"allowed"`
	Remaining int64  `json:"remaining"`
	ResetAt   string `json:"reset_at"`
}

func main() {
	//	fw := limiter.NewFixedWindow()
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	//	fw := limiter.NewFixedWindowRedis(rdb)
	var fw limiter.Limiter = limiter.NewFixedWindowLua(rdb)
	http.HandleFunc("/check", func(w http.ResponseWriter, r *http.Request) {
		var req CheckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid Request", http.StatusBadRequest)
			return
		}

		policy := req.Policy
		config, ok := policies[policy]
		if !ok {
			policy = "default"
			config = policies["default"]
		}

		// allowed, remaining, resetAt := fw.Allow(req.Key, policy, config.Limit, config.WindowSeconds)
		allowed, remaining, resetAt, err := fw.Allow(req.Key, policy, config.Limit, config.WindowSeconds)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if !allowed {
			w.WriteHeader(http.StatusTooManyRequests)
		}
		json.NewEncoder(w).Encode(CheckResponse{
			Allowed:   allowed,
			Remaining: remaining,
			ResetAt:   time.Unix(resetAt, 0).UTC().Format(time.RFC3339),
		})
	})
	fmt.Println("Rate Limiter running on :8080")
	http.ListenAndServe(":8080", nil)
}
