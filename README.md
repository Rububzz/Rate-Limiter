# Distributed Rate Limiter

A high-performance rate limiting service built in Go, evolving from a simple
in-memory implementation to a production-grade distributed system with gRPC,
Redis, and observability.

## Progression

| Version | Tag                   | Description                                         |
| ------- | --------------------- | --------------------------------------------------- |
| v0.1    | `v0.1-in-memory`      | Fixed window rate limiter with in-memory storage    |
| v0.2    | `v0.2-redis`          | Distributed limiter using Redis & Pipelining        |
| v0.3    | `v0.3-lua-atomic`     | Atomic operations via Redis Lua scripting           |
| v0.4    | `v0.4-sliding-window` | Sliding window rate limiter using Redis sorted sets |
| v0.5    | `v0.5-grpc`           | Replace REST with gRPC transport                    |

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

This project uses Docker to manage Redis, while the Go service runs on the host machine.

### 1. Start Redis

```sh
docker-compose up -d
```

### 2. Start Go Server

```sh
go run cmd/server/main.go
```

### 3. Clean up

```sh
docker-compose down
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

### v0.2 - Redis

**_How it works:_**

- Uses Redis Pipelining to group INCR and EXPIRE commands into a single network round-trip, significantly reducing latency.
- State is shared across all service instances, allowing for true distributed rate limiting.
- The windowKey is derived from the Unix timestamp: key:policy:windowID.

**Limitations:**

- Non-Atomicity: Unlike a Redis Transaction (MULTI/EXEC) or a Lua Script, a Pipeline is **not**
  atomic. Commands are sent together, but Redis may execute other clients' commands between ours.
- The "Expiry Race": In a highly concurrent environment, two requests might both call INCR.
  If the process crashes between the INCR and the EXPIRE command in the pipeline, a key
  could theoretically exist without an expiry (though the use of time-slotted keys in this implementation mitigates the long-term impact)
- This will be fixed in `0.3` with the use of lua scripts

### v0.3 — Lua Atomic Script

**How it works:**

- Replaces the pipeline with a Redis Lua script executed via `EVAL`
- `INCR` and `EXPIRE` run as a single atomic operation inside Redis
- Redis is single-threaded — while the Lua script runs, no other command can execute
- The expiry is only set when `count == 1`, meaning the key was just created by this request

**Why this fixes the pipeline bug:**

- In v0.2, a crash between `INCR` and `EXPIRE` leaves the key with no expiry — the user is permanently rate limited
- In v0.3, there is no gap between the two operations — they are one indivisible unit

**Fail open:**

- If Redis is unreachable, requests are allowed through rather than blocking all traffic
- Availability is prioritised over strict enforcement during outages

**Limitations:**

- Fixed window still has a boundary spike problem — a user can exhaust their limit at the
  end of one window and immediately exhaust it again at the start of the next, effectively
  doubling their request rate at the boundary
- This will be addressed in a future version with a Sliding Window algorithm

### v0.4 — Sliding Window

**How it works:**

- Stores the exact timestamp of every request in a Redis Sorted Set (ZSET)
- On each request, entries older than `windowSeconds` are removed via `ZREMRANGEBYSCORE`
- `ZCARD` counts the remaining entries — these are all requests within the current window
- If count is under the limit, the current timestamp is added via `ZADD` and the request is allowed
- The entire sequence runs atomically inside a Lua script

**Why this fixes the boundary spike:**

- Fixed window resets at fixed boundaries — a user can exhaust their limit just before
  a reset and again just after, effectively doubling their request rate
- Sliding window looks back exactly `windowSeconds` from the current moment on every request
  so there is no boundary to exploit

**Key implementation details:**

- Timestamps are stored in milliseconds so two requests within the same second get
  different scores
- Each member includes a random suffix (`timestamp-randomID`) to prevent duplicate
  members overwriting each other under high concurrency
- `ZADD` only runs if the request is allowed — blocked requests are not recorded
- `EXPIRE` is always refreshed so inactive keys are cleaned up automatically by Redis

**Tradeoffs vs Fixed Window:**

- More accurate — no boundary spike possible
- Higher memory usage — stores every request timestamp instead of a single counter
- Slightly more Redis work per request — `ZREMRANGEBYSCORE`, `ZCARD`, `ZADD` vs just `INCR`
- For most use cases the accuracy improvement is worth the tradeoff

**Limitations:**

- At very high request rates the sorted set can grow large within a window
- The `reset_at` field is an approximation — sliding window has no fixed reset point
  since the window rolls continuously with every request

### v0.5 — gRPC Transport

**What changed:**

- Replaced HTTP REST server with a gRPC server
- Defined service contract in `proto/ratelimiter.proto`
- `protoc` generates typed request/response structs and server interface
- No manual JSON encoding/decoding — protobuf handles serialization
- Rate limiting logic in `internals/limiter/` is completely untouched

**Why gRPC over REST:**

- Binary protocol (protobuf) instead of text (JSON) — smaller payloads
- HTTP/2 instead of HTTP/1.1 — multiplexed connections, lower latency
- Strongly typed contract — both sides compile from the same .proto file
- Feels like calling a local function instead of constructing HTTP requests

**How to test:**

- Use grpcurl instead of curl
- grpcurl -plaintext -d '{"key":"user:123","policy":"default"}' localhost:50051 ratelimiter.RateLimiter/Check
