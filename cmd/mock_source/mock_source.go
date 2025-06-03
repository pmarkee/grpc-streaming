package main

import (
	"context"
	"fmt"
	"github.com/pmarkee/grpc-rate-streaming/internal/rates"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	source := rates.NewMockRateSource()
	stream := source.Subscribe(ctx, "USD", "EUR")
	defer source.Unsubscribe(cancel, stream)

	go func() {
		for rate := range stream {
			fmt.Printf("[%s] %s -> %s: %.4f\n",
				rate.Timestamp.Format(time.RFC3339),
				rate.From, rate.To, rate.Rate,
			)
		}
	}()

	<-signals
}
