package main

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/ratelimit"
	"github.com/pmarkee/grpc-rate-streaming/api/proto/pb"
	"github.com/pmarkee/grpc-rate-streaming/internal/interceptors"
	"github.com/pmarkee/grpc-rate-streaming/internal/rates"
	"github.com/pmarkee/grpc-rate-streaming/internal/server"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"net"
	"os"
	"os/signal"
	"time"
)

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

	limiter := &interceptors.PerIpRateLimiter{}

	s := grpc.NewServer(
		grpc.Creds(creds),
		grpc.ChainStreamInterceptor(
			logging.StreamServerInterceptor(InterceptorLogger(log.Logger)),
			ratelimit.StreamServerInterceptor(limiter),
			interceptors.ApiKeyInterceptor,
		),
	)

	// Health checking
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(s, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	go healthCheckWorker(ctx, source, healthServer)

	reflection.Register(s)
	pb.RegisterCurrencyServer(s, server.NewCurrencyServer(ctx, source))
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
