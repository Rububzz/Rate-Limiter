package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/Rububzz/Rate-Limiter/internals/limiter"
	pb "github.com/Rububzz/Rate-Limiter/proto"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// policies defined server side — caller never sets limits
type LimitConfig struct {
	Limit         int64
	WindowSeconds int64
}

var policies = map[string]LimitConfig{
	"default":       {Limit: 3, WindowSeconds: 60},
	"premium":       {Limit: 10, WindowSeconds: 60},
	"auth_endpoint": {Limit: 5, WindowSeconds: 60},
}

// your server struct — implements RateLimiterServer
type rateLimiterServer struct {
	pb.UnimplementedRateLimiterServer // satisfies mustEmbedUnimplementedRateLimiterServer
	fw                                limiter.Limiter
}

// Check is the only method you need to implement
func (s *rateLimiterServer) Check(ctx context.Context, req *pb.CheckRequest) (*pb.CheckResponse, error) {
	// validate request
	if req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}

	// look up policy server side
	policy := req.Policy
	cfg, ok := policies[policy]
	if !ok {
		policy = "default"
		cfg = policies["default"]
	}

	// call the rate limiter — same as before
	allowed, remaining, resetAt, err := s.fw.Allow(req.Key, policy, cfg.Limit, cfg.WindowSeconds)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &pb.CheckResponse{
		Allowed:   allowed,
		Remaining: remaining,
		ResetAt:   time.Unix(resetAt, 0).UTC().Format(time.RFC3339),
	}, nil
}

func main() {
	// connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// use sliding window — swap this line to change algorithm
	fw := limiter.NewSlidingWindowLua(rdb)

	// create your server
	srv := &rateLimiterServer{fw: fw}

	// create the gRPC server
	grpcServer := grpc.NewServer()

	// wire your implementation to the gRPC server
	pb.RegisterRateLimiterServer(grpcServer, srv)
	reflection.Register(grpcServer)

	// listen on port 50051
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	fmt.Println("Rate Limiter gRPC server running on :50051")

	// start serving
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
