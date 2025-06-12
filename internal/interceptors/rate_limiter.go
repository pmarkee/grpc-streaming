package interceptors

import (
	"context"
	"golang.org/x/time/rate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"net"
	"strings"
	"sync"
	"time"
)

var (
	ipLimiters sync.Map
	globalRate = rate.Every(1 * time.Second)
	burstSize  = 10
)

func getLimiter(ip string) *rate.Limiter {
	limiter, _ := ipLimiters.LoadOrStore(ip, rate.NewLimiter(globalRate, burstSize))
	return limiter.(*rate.Limiter)
}

func extractIP(ctx context.Context) (string, error) {
	// From gRPC peer (direct connection)
	p, ok := peer.FromContext(ctx)
	if ok {
		host, _, err := net.SplitHostPort(p.Addr.String())
		if err == nil {
			return host, nil
		}
	}

	// From headers (e.g., behind proxy/LB)
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if xff := md.Get("x-forwarded-for"); len(xff) > 0 {
			return strings.Split(xff[0], ",")[0], nil
		}
		if xrip := md.Get("x-real-ip"); len(xrip) > 0 {
			return xrip[0], nil
		}
	}

	return "", status.Error(codes.InvalidArgument, "IP not found")
}

type PerIpRateLimiter struct{}

func (p *PerIpRateLimiter) Limit(ctx context.Context) error {
	ip, err := extractIP(ctx)
	if err != nil {
		return err
	}

	limiter := getLimiter(ip)
	if !limiter.Allow() {
		return status.Errorf(
			codes.ResourceExhausted,
			"stream rate limit exceeded for IP %s (try again in %v)",
			ip,
			limiter.Reserve().Delay(),
		)
	}

	return nil
}
