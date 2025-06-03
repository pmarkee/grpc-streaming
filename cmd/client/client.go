package main

import (
	"context"
	"github.com/pmarkee/grpc-rate-streaming/api/proto/pb"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"io"
	"os"
	"os/signal"
	"sync"
	"time"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly})
	ctx, cancel := context.WithCancel(context.Background())

	creds, err := credentials.NewClientTLSFromFile("certs/server.crt", "localhost")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load TLS credentials")
	}

	grpcClient, err := grpc.NewClient("localhost:8080", grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatal().Err(err).Msg("could not establish gRPC client")
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	client := pb.NewCurrencyClient(grpcClient)
	pair := &pb.CurrencyPair{
		From: "USD",
		To:   "EUR",
	}
	stream, err := client.Subscribe(ctx, pair)
	if err != nil {
		log.Fatal().Err(err).Msg("calling Subscribe failed")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go receiver(stream, &wg)

	go func() {
		<-signals
		log.Info().Msg("user interrupt")
		cancel()
	}()

	wg.Wait()
	log.Info().Msg("client shut down gracefully")
}

func receiver(stream grpc.ServerStreamingClient[pb.CurrencyExchangeRate], wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		rate, err := stream.Recv()
		if err == io.EOF {
			log.Info().Msg("stream closed by server")
			break
		} else if status.Code(err) == codes.Canceled {
			log.Info().Msg("receiver, shutting down, context canceled")
			break
		} else if err != nil {
			log.Error().Err(err).Msg("stream closed for unknown reason")
			break
		}
		datetime := rate.Timestamp.AsTime().Format(time.DateTime)
		log.Info().Msgf("%s: %s -> %s = %.2f", datetime, rate.From, rate.To, rate.Rate)
	}
}
