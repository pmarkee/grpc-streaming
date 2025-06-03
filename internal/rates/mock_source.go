package rates

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

type RateUpdate struct {
	From      string
	To        string
	Rate      float32
	Timestamp time.Time
}

type MockRateSource struct {
	streams map[<-chan RateUpdate]struct{} // A Set would be more appropriate, but better not add a dependency just for this.
	mu      sync.Mutex
}

// NewMockRateSource creates and starts a mock data source with one stream per currency pair.
func NewMockRateSource() *MockRateSource {
	return &MockRateSource{streams: make(map[<-chan RateUpdate]struct{})}
}

// Subscribe returns a read-only channel for the given currency pair.
func (s *MockRateSource) Subscribe(ctx context.Context, from, to string) <-chan RateUpdate {
	s.mu.Lock()
	defer s.mu.Unlock()

	stream := startRateStream(ctx, from, to)
	s.streams[stream] = struct{}{}

	return stream
}

// Unsubscribe cancels the context for the given stream and removes it from the list of active streams.
func (s *MockRateSource) Unsubscribe(cancel context.CancelFunc, stream <-chan RateUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cancel()
	delete(s.streams, stream)
}

// startRateStream starts a goroutine that emits rate updates every second.
func startRateStream(ctx context.Context, from, to string) <-chan RateUpdate {
	ch := make(chan RateUpdate)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		defer close(ch)

		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				ch <- RateUpdate{
					From:      from,
					To:        to,
					Rate:      randomRate(from, to), // Different clients will get different rates, but it doesn't matter for this demo.
					Timestamp: t.UTC(),
				}
			}
		}
	}()

	return ch
}

func pairKey(from, to string) string {
	return from + "_" + to
}

// randomRate returns a mock exchange rate
func randomRate(from, to string) float32 {
	base := rand.Float32()*0.2 + 0.9 // Between 0.9 and 1.1
	switch pairKey(from, to) {
	case "USD_EUR":
		return base * 1.05
	case "USD_JPY":
		return base * 110
	case "EUR_GBP":
		return base * 0.85
	case "BTC_USD":
		return base * 30000
	default:
		return base
	}
}
