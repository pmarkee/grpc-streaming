package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/pmarkee/grpc-rate-streaming/api/proto/pb"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"io"
	"maps"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

type Config struct {
	From      string
	To        string
	CACert    string
	ServerURL string
}

var supportedPairs = map[string]struct{}{
	"USD/EUR": {},
	"EUR/GBP": {},
	"USD/JPY": {},
	"BTC/USD": {},
}

func isValidPair(from, to string) bool {
	key := from + "/" + to
	_, ok := supportedPairs[key]
	return ok
}

func parseFlags() *Config {
	from := flag.String("from", "", "Base currency (e.g. USD)")
	to := flag.String("to", "", "Target currency (e.g. EUR)")
	caCert := flag.String("ca-cert", "./certs/server.crt", "Path to server CA certificate")
	serverUrl := flag.String("server-url", "localhost:8080", "URL to gRPC server")

	flag.Parse()

	if *from == "" || *to == "" {
		fmt.Println("Both --from and --to arguments are required.")
		flag.Usage()
		return nil
	}

	if !isValidPair(*from, *to) {
		var spList []string
		for key := range maps.Keys(supportedPairs) {
			spList = append(spList, key)
		}
		fmt.Printf("Invalid currency pair, supported pairs (from/to) are: %s\n", strings.Join(spList, ", "))
		return nil
	}

	return &Config{
		From:      *from,
		To:        *to,
		CACert:    *caCert,
		ServerURL: *serverUrl,
	}
}

func main() {
	config := parseFlags()
	if config == nil {
		os.Exit(1)
	}

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly})
	ctx, cancel := context.WithCancel(context.Background())

	creds, err := credentials.NewClientTLSFromFile(config.CACert, "")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load TLS credentials")
	}

	grpcClient, err := grpc.NewClient(config.ServerURL, grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatal().Err(err).Msg("could not establish gRPC client")
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	client := pb.NewCurrencyClient(grpcClient)
	pair := &pb.CurrencyPair{
		From: config.From,
		To:   config.To,
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
