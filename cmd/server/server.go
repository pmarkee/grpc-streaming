package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/ratelimit"
	"github.com/pmarkee/grpc-rate-streaming/api/proto/pb"
	"github.com/pmarkee/grpc-rate-streaming/internal/rates"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

type server struct {
	pb.UnimplementedCurrencyServer
	ctx    context.Context
	source *rates.MockRateSource
}

func (s *server) Subscribe(in *pb.CurrencyPair, streamingServer grpc.ServerStreamingServer[pb.CurrencyExchangeRate]) error {
	if in.From == "" || in.To == "" {
		return status.Error(codes.InvalidArgument, "both 'from' and 'to' fields are required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	stream := s.source.Subscribe(ctx, in.From, in.To)
	defer s.source.Unsubscribe(cancel, stream)

	streamCtx := streamingServer.Context()
	mainCtx := s.ctx
	for {
		select {
		case <-streamCtx.Done():
			return handleConnShutdown(streamCtx.Err())
		case <-mainCtx.Done():
			return nil
		case xchgRate := <-stream:
			msg := &pb.CurrencyExchangeRate{
				Rate:      xchgRate.Rate,
				From:      xchgRate.From,
				To:        xchgRate.To,
				Timestamp: timestamppb.New(xchgRate.Timestamp),
			}
			if err := streamingServer.Send(msg); err != nil {
				return handleConnShutdown(err)
			}
		}
	}
}

func handleConnShutdown(err error) error {
	if errors.Is(err, context.Canceled) {
		log.Debug().Err(err).Msg("connection shut down by client")
		return nil
	}
	if err != nil {
		log.Error().Err(err).Msg("connection shut down unexpectedly")
		return err
	}
	return nil
}

// InterceptorLogger adapts zerolog logger to interceptor logger.
// from https://github.com/grpc-ecosystem/go-grpc-middleware/blob/main/interceptors/logging/examples/zerolog/example_test.go
func InterceptorLogger(l zerolog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		l := l.With().Fields(fields).Logger()

		switch lvl {
		case logging.LevelDebug:
			l.Debug().Msg(msg)
		case logging.LevelInfo:
			l.Info().Msg(msg)
		case logging.LevelWarn:
			l.Warn().Msg(msg)
		case logging.LevelError:
			l.Error().Msg(msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}

var apiKeys = map[string]bool{"super-secret-api-key": true}

// apiKeyInterceptor checks for API key validity
func apiKeyInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	md, ok := metadata.FromIncomingContext(ss.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}

	apiKeyHeaders := md.Get("x-api-key")
	if len(apiKeyHeaders) == 0 || !apiKeys[apiKeyHeaders[0]] {
		return status.Error(codes.Unauthenticated, "invalid API key")
	}

	return handler(srv, ss)
}

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

type perIpRateLimiter struct{}

func (p *perIpRateLimiter) Limit(ctx context.Context) error {
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

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly})

	ctx, cancel := context.WithCancel(context.Background())

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	source := rates.NewMockRateSource()

	tcpListener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Error().Err(err).Msg("failed to create TCP listener")
	}

	creds, err := credentials.NewServerTLSFromFile("certs/server.crt", "certs/server.key")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load TLS credentials")
	}

	limiter := &perIpRateLimiter{}

	s := grpc.NewServer(
		grpc.Creds(creds),
		grpc.ChainStreamInterceptor(
			logging.StreamServerInterceptor(InterceptorLogger(log.Logger)),
			ratelimit.StreamServerInterceptor(limiter),
			apiKeyInterceptor,
		),
	)

	// Health checking
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(s, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	go healthCheckWorker(ctx, source, healthServer)

	reflection.Register(s)
	pb.RegisterCurrencyServer(s, &server{source: source, ctx: ctx})
	go func() {
		log.Info().Msg("gRPC server starting up with TLS on port 8080")
		if err := s.Serve(tcpListener); err != nil {
			log.Fatal().Err(err).Msg("failed to start grpc server")
		}
	}()

	<-signals
	log.Info().Msg("shutting down")
	cancel()
	s.GracefulStop()
}

func healthCheckWorker(ctx context.Context, source *rates.MockRateSource, healthServer *health.Server) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			healthStatus := healthpb.HealthCheckResponse_SERVING
			if !source.Healthy() {
				healthStatus = healthpb.HealthCheckResponse_NOT_SERVING
			}
			healthServer.SetServingStatus("currency.Currency", healthStatus)
		}
	}
}
