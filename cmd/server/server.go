package main

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/pmarkee/grpc-rate-streaming/api/proto/pb"
	"github.com/pmarkee/grpc-rate-streaming/internal/rates"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"net"
	"os"
	"os/signal"
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

	stream, ok := s.source.GetStream(in.From, in.To)
	if !ok {
		return status.Error(codes.NotFound, "stream does not exist")
	}

	streamCtx := streamingServer.Context()
	mainCtx := s.ctx
	for {
		select {
		case <-streamCtx.Done():
			return streamCtx.Err() // this will be `context.Canceled` or `context.DeadlineExceeded`
		case <-mainCtx.Done():
			return status.Error(codes.Canceled, "connection closed by server")
		case rate := <-stream:
			msg := &pb.CurrencyExchangeRate{
				Rate:      rate.Rate,
				From:      rate.From,
				To:        rate.To,
				Timestamp: timestamppb.New(rate.Timestamp),
			}
			err := streamingServer.Send(msg)
			if err != nil {
				log.Printf("error sending to client: %v", err)
				return err
			}
		}
	}
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

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly})

	ctx, cancel := context.WithCancel(context.Background())

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	source := rates.NewMockRateSource(ctx)

	tcpListener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Error().Err(err).Msg("failed to create TCP listener")
	}

	s := grpc.NewServer(grpc.ChainStreamInterceptor(logging.StreamServerInterceptor(InterceptorLogger(log.Logger))))
	reflection.Register(s)
	pb.RegisterCurrencyServer(s, &server{source: source, ctx: ctx})
	go func() {
		if err := s.Serve(tcpListener); err != nil {
			log.Fatal().Err(err).Msg("failed to start grpc server")
		}
	}()

	<-signals
	fmt.Println("\nShutting down...")
	cancel()
	s.GracefulStop()
}
