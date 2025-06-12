package server

import (
	"context"
	"errors"
	"github.com/pmarkee/grpc-rate-streaming/api/proto/pb"
	"github.com/pmarkee/grpc-rate-streaming/internal/rates"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type CurrencyServer struct {
	pb.UnimplementedCurrencyServer
	ctx    context.Context
	source *rates.MockRateSource
}

func NewCurrencyServer(ctx context.Context, source *rates.MockRateSource) *CurrencyServer {
	return &CurrencyServer{ctx: ctx, source: source}
}

func (s *CurrencyServer) Subscribe(in *pb.CurrencyPair, streamingServer grpc.ServerStreamingServer[pb.CurrencyExchangeRate]) error {
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
