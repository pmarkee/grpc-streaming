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
	Rate      float64
	Timestamp time.Time
}

type RateStream <-chan RateUpdate

type MockRateSource struct {
	ctx     context.Context
	streams map[string]RateStream
	mu      sync.Mutex
}

// NewMockRateSource creates and starts a mock data source with one stream per currency pair.
func NewMockRateSource(ctx context.Context) *MockRateSource {
	return &MockRateSource{ctx: ctx, streams: make(map[string]RateStream)}
}

// GetStream returns a read-only channel for the given currency pair.
func (s *MockRateSource) GetStream(from, to string) (RateStream, bool) {
	key := pairKey(from, to)

	s.mu.Lock()
	defer s.mu.Unlock()

	if stream, ok := s.streams[key]; ok {
		return stream, true
	}

	stream := startRateStream(s.ctx, from, to)
	s.streams[key] = stream
	return stream, true
}

// startRateStream starts a goroutine that emits rate updates every 2 seconds.
func startRateStream(ctx context.Context, from, to string) RateStream {
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
					Rate:      randomRate(from, to),
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
func randomRate(from, to string) float64 {
	base := rand.Float64()*0.2 + 0.9 // Between 0.9 and 1.1
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
