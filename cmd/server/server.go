package main

import (
	"context"
	"fmt"
	"github.com/pmarkee/grpc-rate-streaming/api/proto/pb"
	"github.com/pmarkee/grpc-rate-streaming/internal/rates"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
	"net"
	"os"
	"os/signal"
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
			log.Println("connection closed by client")
			return streamCtx.Err() // this will be `context.Canceled` or `context.DeadlineExceeded`
		case <-mainCtx.Done():
			log.Println("connection closed by server")
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

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	source := rates.NewMockRateSource(ctx)

	tcpListener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalln("failed to create TCP listener", err)
	}

	s := grpc.NewServer()
	reflection.Register(s)
	pb.RegisterCurrencyServer(s, &server{source: source, ctx: ctx})
	go func() {
		if err := s.Serve(tcpListener); err != nil {
			log.Fatalln("failed to start grpc server", err)
		}
	}()

	<-signals
	fmt.Println("\nShutting down...")
	cancel()
	s.GracefulStop()
}
