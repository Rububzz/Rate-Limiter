# Distributed Rate Limiter

A high-performance rate limiting service built in Go, evolving from a simple
in-memory implementation to a production-grade distributed system with gRPC,
Redis, and observability.

## Progression

| Version | Tag              | Description                                      |
| ------- | ---------------- | ------------------------------------------------ |
| v0.1    | `v0.1-in-memory` | Fixed window rate limiter with in-memory storage |

## Architecture

```
Client Request
     │
     ▼
API Gateway
     │  extracts user identity from auth token
     │  calls rate limiter with real key
     ▼
Rate Limiter ←── key = "user:123", policy = "default"
     │
     ▼
Counter for "user:123:default:bucket"
```

## Policies

| Policy          | Limit       | Window     |
| --------------- | ----------- | ---------- |
| `default`       | 3 requests  | 60 seconds |
| `premium`       | 10 requests | 60 seconds |
| `auth_endpoint` | 5 requests  | 60 seconds |

## Running Locally

```sh
go run cmd/server/main.go
```

## API

### POST /check

Request:

```json
{
  "key": "user:123",
  "policy": "default"
}
```

Response:

```json
{
  "allowed": true,
  "remaining": 2,
  "reset_at": "2026-04-01T10:20:00Z"
}
```

| Field       | Type   | Description                              |
| ----------- | ------ | ---------------------------------------- |
| `allowed`   | bool   | Whether the request is permitted         |
| `remaining` | int    | Remaining requests in the current window |
| `reset_at`  | string | UTC timestamp when the window resets     |

Returns `429 Too Many Requests` when `allowed` is false.

## Testing

Run 4 requests as the same user to trigger the rate limit:

```sh
for i in 1 2 3 4; do
  curl -s -X POST http://localhost:8080/check \
    -H "Content-Type: application/json" \
    -d '{"key":"user:123","policy":"default"}' | jq .
done
```

Verify different users have isolated counters:

```sh
curl -s -X POST http://localhost:8080/check \
  -H "Content-Type: application/json" \
  -d '{"key":"user:456","policy":"default"}' | jq .
```

`user:456` should return `remaining: 2` even after `user:123` is blocked.

## Versions

### v0.1 — In-Memory Fixed Window

**How it works:**

- Each request increments a counter scoped to `key + policy + time bucket`
- The time bucket is calculated as `now / windowSeconds` so all requests
  within the same window share a counter
- `sync.Mutex` ensures concurrent requests don't produce a race condition
- Counter resets naturally when the time bucket changes — no manual cleanup needed

**Limitations:**

- State is in-memory — counters are lost on restart
- Not distributed — running multiple instances means each has its own counter,
  allowing users to exceed the limit across instances
- These will be solved in `v0.2` with Redis
