// betfair/prices.go
//
// Live prices from the Exchange Stream. The catalogue picks the markets, one
// goroutine applies the stream's changes, and readers get a lock-free
// snapshot per market.

package betfair

import (
	"context"
	"fmt"
	"maps"
	"time"

	"pegasus_suite/betting/betfair/internal/exchange"
	"pegasus_suite/logger"
)

// Depth is how many price levels each side of the book carries.
const Depth = 3

const (
	minBackoff = time.Second
	maxBackoff = 30 * time.Second
)

type Level struct {
	Price float64
	Size  float64
}

// RunnerPrices is one runner's book, best price first on each side.
type RunnerPrices struct {
	Back [Depth]Level
	Lay  [Depth]Level
	LTP  float64
}

type marketPrices = map[int64]RunnerPrices

// Runner returns the latest prices for a selection in a streamed market.
func (bc *Client) Runner(marketID string, selectionID int64) (RunnerPrices, bool) {
	m, ok := bc.prices.Load(marketID)
	if !ok {
		return RunnerPrices{}, false
	}
	r, ok := m.(marketPrices)[selectionID]
	return r, ok
}

// StartRunnerUpdates streams prices for the catalogue's markets until ctx ends, reconnecting as
// needed. onClosed is called with each market the stream reports CLOSED.
func (bc *Client) StartRunnerUpdates(ctx context.Context, onClosed func(marketID string)) {
	go func() {
		var ids []string
		var initialClk, clk string
		backoff := minBackoff
		for ctx.Err() == nil {
			started := time.Now()
			err := bc.stream(ctx, &ids, &initialClk, &clk, onClosed)
			if ctx.Err() != nil {
				return
			}
			logger.Warn(logger.Log{Message: fmt.Sprintf("betfair stream dropped, reconnecting in %v error=%v", backoff, err)})
			if time.Since(started) > maxBackoff {
				backoff = minBackoff
			}
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
			}
			backoff = min(backoff*2, maxBackoff)
		}
	}()
}

// subscribe hands the stream a new market set, replacing any it has not picked up yet.
func (bc *Client) subscribe(ids []string) {
	select {
	case <-bc.subscription:
	default:
	}
	bc.subscription <- ids
}

// stream runs one connection until it fails or ctx ends. ids and the clocks outlive it so the
// next connection resumes where this one stopped.
func (bc *Client) stream(ctx context.Context, ids *[]string, initialClk, clk *string, onClosed func(string)) error {
	for len(*ids) == 0 {
		select {
		case *ids = <-bc.subscription:
		case <-ctx.Done():
			return nil
		}
	}

	s, err := bc.api.DialStream()
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.Subscribe(*ids, Depth, *initialClk, *clk); err != nil {
		return err
	}

	messages := make(chan exchange.StreamMessage, 64)
	failed := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			msg, err := s.Read()
			if err != nil {
				failed <- err
				return
			}
			select {
			case messages <- msg:
			case <-done:
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-failed:
			return err
		case next := <-bc.subscription:
			bc.dropMarketsNotIn(next)
			*ids, *initialClk, *clk = next, "", ""
			if err := s.Subscribe(next, Depth, "", ""); err != nil {
				return err
			}
		case msg := <-messages:
			if msg.InitialClk != "" {
				*initialClk = msg.InitialClk
			}
			if msg.Clk != "" {
				*clk = msg.Clk
			}
			for _, mc := range msg.MC {
				bc.applyMarketChange(mc, onClosed)
			}
		}
	}
}

// applyMarketChange folds one market's changes into a new snapshot, or drops the market when it closes.
func (bc *Client) applyMarketChange(mc exchange.MarketChange, onClosed func(string)) {
	if mc.MarketDefinition != nil && mc.MarketDefinition.Status == exchange.MarketStatusClosed {
		bc.prices.Delete(mc.ID)
		onClosed(mc.ID)
		return
	}
	if len(mc.RC) == 0 {
		return
	}

	next := make(marketPrices, len(mc.RC))
	if old, ok := bc.prices.Load(mc.ID); ok && !mc.Img {
		next = maps.Clone(old.(marketPrices))
	}
	for _, rc := range mc.RC {
		r := next[rc.ID]
		if rc.LTP != 0 {
			r.LTP = rc.LTP
		}
		applyLevels(&r.Back, rc.BATB)
		applyLevels(&r.Lay, rc.BATL)
		next[rc.ID] = r
	}
	bc.prices.Store(mc.ID, next)
}

func applyLevels(book *[Depth]Level, changes [][3]float64) {
	for _, c := range changes {
		level := int(c[0])
		if level < 0 || level >= Depth {
			continue
		}
		if c[2] == 0 {
			book[level] = Level{}
			continue
		}
		book[level] = Level{Price: c[1], Size: c[2]}
	}
}

// dropMarketsNotIn forgets the prices of every streamed market missing from ids.
func (bc *Client) dropMarketsNotIn(ids []string) {
	keep := make(map[string]bool, len(ids))
	for _, id := range ids {
		keep[id] = true
	}
	bc.prices.Range(func(key, _ any) bool {
		if id := key.(string); !keep[id] {
			bc.prices.Delete(id)
		}
		return true
	})
}
