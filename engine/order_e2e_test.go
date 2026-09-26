package engine

// End-to-end tests of the order path: engine.Place → dedupe → sizing → a mock
// bookmaker session, with a fake Betfair price book. Run with -race.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"pegasus_suite/betting"
	"pegasus_suite/betting/betfair"
	"pegasus_suite/betting/betmatic"
)

// mockBook is a bookmaker session that records every request it is sent.
type mockBook struct {
	mu       sync.Mutex
	sent     []betting.BetRequest
	failNext int           // this many requests are rejected before any is accepted
	delay    time.Duration // held before answering, to widen races
}

func (m *mockBook) PlaceBet(req betting.BetRequest) (string, error) {
	time.Sleep(m.delay)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, req)
	if m.failNext > 0 {
		m.failNext--
		return "", errors.New("rejected by bookmaker")
	}
	return fmt.Sprintf("bet-%d", len(m.sent)), nil
}

func (m *mockBook) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

func (m *mockBook) last() betting.BetRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sent[len(m.sent)-1]
}

// fakeBooks serves fixed Betfair prices in place of the admin stream.
type fakeBooks struct {
	mu     sync.Mutex
	prices map[string]map[int64]betfair.RunnerPrices
}

func (f *fakeBooks) GetRace(betfair.RacingCode, string, string, int) *betfair.Race { return nil }
func (f *fakeBooks) Close()                                                        {}

func (f *fakeBooks) Runner(marketID string, selectionID int64) (betfair.RunnerPrices, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.prices[marketID][selectionID]
	return p, ok
}

func (f *fakeBooks) set(marketID string, selectionID int64, back, lay, ltp float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.prices == nil {
		f.prices = map[string]map[int64]betfair.RunnerPrices{}
	}
	if f.prices[marketID] == nil {
		f.prices[marketID] = map[int64]betfair.RunnerPrices{}
	}
	var r betfair.RunnerPrices
	r.Back[0] = betfair.Level{Price: back, Size: 100}
	r.Lay[0] = betfair.Level{Price: lay, Size: 100}
	r.LTP = ltp
	f.prices[marketID][selectionID] = r
}

const market = "1.234"

type harness struct {
	eng   *Engine
	books *fakeBooks
}

func newHarness(t *testing.T) *harness {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h := &harness{eng: New(ctx), books: &fakeBooks{}}
	h.eng.useBooks(h.books)
	return h
}

// account binds mock sessions to a process the way Engine.Account does, sharing its user's claims.
func (h *harness) account(app, user, process string) (*Account, *mockBook, *mockBook) {
	bm, bf := &mockBook{}, &mockBook{}
	c, _ := h.eng.users.LoadOrStore(user, newClaims())
	a := &Account{App: app, UserID: user, ProcessID: process, BotID: "bot-" + user, Bookmakers: []string{"3", "7"}, betmatic: bm, betfair: bf, claims: c.(*claims)}
	return a, bm, bf
}

func win(a *Account, runner int) Order {
	return Order{
		Account:     a,
		Race:        Race{Date: "2026-09-26", Venue: "EAGLE FARM", Metro: true, Code: betmatic.THOROUGHBRED, Number: 3, MarketID: market},
		Runner:      runner,
		SelectionID: int64(1000 + runner),
		Side:        BetmaticWin,
		Unit:        1,
		Stake: Stake{
			Betmatic: BetmaticStake{WinStake: 10, MinOdds: 1.5, MaxOdds: 20},
			Betfair:  BetfairStake{BackStake: 50, LayStake: 30, MinOdds: 1.5, MaxOdds: 20},
		},
	}
}

func with(o Order, side Side) Order { o.Side = side; return o }

// placeAll places every order at once and waits for them all.
func placeAll(eng *Engine, orders ...Order) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, o := range orders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			eng.Place(o)
		}()
	}
	close(start)
	wg.Wait()
}

func repeat(o Order, n int) []Order {
	out := make([]Order, n)
	for i := range out {
		out[i] = o
	}
	return out
}

func TestE2EConcurrentDuplicatesReachTheBookmakerOnce(t *testing.T) {
	h := newHarness(t)
	a, bm, _ := h.account("pegasus", "u1", "p1")
	bm.delay = 5 * time.Millisecond

	placeAll(h.eng, repeat(win(a, 4), 200)...)

	if n := bm.count(); n != 1 {
		t.Fatalf("200 identical orders sent %d requests, want 1", n)
	}
}

func TestE2EEachProcessOfOneAppBetsOnce(t *testing.T) {
	h := newHarness(t)
	var orders []Order
	var books []*mockBook
	for p := range 5 {
		a, bm, _ := h.account("davo", "u1", fmt.Sprintf("p%d", p))
		books = append(books, bm)
		orders = append(orders, repeat(win(a, 4), 20)...)
	}

	placeAll(h.eng, orders...)

	for i, bm := range books {
		if n := bm.count(); n != 1 {
			t.Errorf("process %d sent %d requests, want 1", i, n)
		}
	}
}

func TestE2EFirstAppToBetOwnsTheRunner(t *testing.T) {
	for round := range 50 {
		h := newHarness(t)
		peg, pegBook, _ := h.account("pegasus", "u1", "peg")
		davo, davoBook, _ := h.account("davo", "u1", "davo")

		placeAll(h.eng, append(repeat(win(peg, 4), 10), repeat(win(davo, 4), 10)...)...)

		if p, d := pegBook.count(), davoBook.count(); p+d != 1 {
			t.Fatalf("round %d: pegasus sent %d, davo sent %d; want exactly one bet between them", round, p, d)
		}
	}
}

func TestE2EOneProcessBetsEachProviderOnceOnARunner(t *testing.T) {
	h := newHarness(t)
	h.books.set(market, 1004, 4.0, 4.2, 4.1)
	a, bm, bf := h.account("pegasus", "u1", "p1")
	o := win(a, 4)

	h.eng.Place(with(o, BetmaticWin))
	h.eng.Place(with(o, BetfairBack))
	h.eng.Place(with(o, BetmaticPlace)) // betmatic again
	h.eng.Place(with(o, BetfairLay))    // betfair again

	if bm.count() != 1 || bf.count() != 1 {
		t.Fatalf("betmatic sent %d, betfair sent %d; want 1 each", bm.count(), bf.count())
	}
	if req := bf.last().(betfair.BSPBetRequest); req.Side != betfair.SideBack {
		t.Fatalf("betfair bet was %s, want the first (BACK)", req.Side)
	}
}

func TestE2ERejectedBetIsNeverRetried(t *testing.T) {
	h := newHarness(t)
	peg, pegBook, _ := h.account("pegasus", "u1", "peg")
	davo, davoBook, _ := h.account("davo", "u1", "davo")
	pegBook.failNext = 1

	h.eng.Place(win(peg, 4))                     // rejected by the bookmaker
	placeAll(h.eng, repeat(win(peg, 4), 50)...)  // no retry from the same process
	placeAll(h.eng, repeat(win(davo, 4), 50)...) // nor from another app

	if pegBook.count() != 1 || davoBook.count() != 0 {
		t.Fatalf("pegasus sent %d, davo sent %d; want only the one rejected request", pegBook.count(), davoBook.count())
	}
}

func TestE2ERejectedBetLeavesOtherProvidersOpen(t *testing.T) {
	h := newHarness(t)
	h.books.set(market, 1004, 4.0, 4.2, 4.1)
	a, bm, bf := h.account("pegasus", "u1", "p1")
	bm.failNext = 1

	h.eng.Place(with(win(a, 4), BetmaticWin))
	h.eng.Place(with(win(a, 4), BetfairBack))

	if bm.count() != 1 || bf.count() != 1 {
		t.Fatalf("betmatic sent %d, betfair sent %d; want 1 each", bm.count(), bf.count())
	}
}

func TestE2ESkippedBetfairBetFreesTheRunner(t *testing.T) {
	h := newHarness(t)
	a, _, bf := h.account("pegasus", "u1", "p1")
	o := with(win(a, 4), BetfairBack)

	h.eng.Place(o) // no prices yet: skipped, not sent
	if bf.count() != 0 {
		t.Fatal("a bet was sent with no prices")
	}

	h.books.set(market, 1004, 4.0, 4.2, 4.1)
	h.eng.Place(o)
	h.eng.Place(o)
	if bf.count() != 1 {
		t.Fatalf("sent %d requests once prices arrived, want 1", bf.count())
	}

	req := bf.last().(betfair.BSPBetRequest)
	if req.OrderRef != "EAGLEFARM:R3:4:20260926" || req.StrategyRef != "pegasus" || req.MarketID != market || req.SelectionID != 1004 {
		t.Errorf("refs and ids = %+v", req)
	}
	if req.LimitPrice != 3.6 || req.Liability != 16.67 {
		t.Errorf("limit %.2f liability %.2f, want 3.60 (best back less 10%%) and 16.67 ($50 to win at 4.00)", req.LimitPrice, req.Liability)
	}
}

func TestE2EMarketCloseFreesItsRunners(t *testing.T) {
	h := newHarness(t)
	a, bm, _ := h.account("pegasus", "u1", "p1")

	h.eng.Place(win(a, 4))
	h.eng.closeMarket("1.999") // another market
	h.eng.Place(win(a, 4))
	if bm.count() != 1 {
		t.Fatalf("another market's close freed this runner: sent %d", bm.count())
	}

	h.eng.closeMarket(market)
	h.eng.Place(win(a, 4))
	if bm.count() != 2 {
		t.Fatalf("sent %d after the market closed, want 2", bm.count())
	}
}

func TestE2EClaimsExpire(t *testing.T) {
	h := newHarness(t)
	a, bm, _ := h.account("davo", "u1", "p1")
	o := win(a, 4)
	o.Race.MarketID = ""

	h.eng.Place(o)
	for _, c := range h.eng.allClaims() {
		c.drop("", time.Now().Add(-claimTTL))
	}
	h.eng.Place(o)
	if bm.count() != 1 {
		t.Fatalf("a claim younger than %v expired", claimTTL)
	}

	for _, c := range h.eng.allClaims() {
		c.drop("", time.Now().Add(time.Second))
	}
	h.eng.Place(o)
	if bm.count() != 2 {
		t.Fatalf("sent %d after expiry, want 2", bm.count())
	}
}

func TestE2EOtherUsersRunnersAndRacesAreIndependent(t *testing.T) {
	h := newHarness(t)
	u1, u1Book, _ := h.account("pegasus", "u1", "p1")
	u2, u2Book, _ := h.account("davo", "u2", "p1")

	tomorrow := win(u1, 4)
	tomorrow.Race.Date = "2026-09-27"
	nextRace := win(u1, 4)
	nextRace.Race.Number = 4

	placeAll(h.eng, win(u1, 4), win(u1, 5), tomorrow, nextRace, win(u2, 4))

	if u1Book.count() != 4 || u2Book.count() != 1 {
		t.Fatalf("u1 sent %d (want 4), u2 sent %d (want 1)", u1Book.count(), u2Book.count())
	}
}

func TestE2EBetmaticRequests(t *testing.T) {
	h := newHarness(t)
	a, bm, _ := h.account("pegasus", "u1", "p1")

	mbl := win(a, 4)
	mbl.Stake.Betmatic.WinMBL = true
	mbl.Unit = 0.5
	h.eng.Place(mbl)
	req := bm.last().(betmatic.NotificationRequest)
	if req.Type != betmatic.FIXED_PROFIT || req.TargetProfit != 1000 || req.Label != "PEGASUS" || req.Competition != "EAGLE FARM" ||
		req.EventNumber != 3 || req.Selection != 4 || req.TargetBot != "bot-u1" || req.BookiesOverride != "3,7" {
		t.Errorf("mbl request = %+v, want $2000 metro thoroughbred MBL x 0.5 unit", req)
	}

	d, dBook, _ := h.account("davo", "u2", "p1")
	cash := with(win(d, 6), BetmaticPlace)
	cash.Stake = Stake{Betmatic: BetmaticStake{WinStake: 25, MinOdds: 3, Cash: true}}
	cash.Unit = 2
	h.eng.Place(cash)
	req = dBook.last().(betmatic.NotificationRequest)
	if req.Type != betmatic.HIGH_ODDS_FIRST || req.Market != betmatic.FIXED_PLACE || req.Stake != 50 || req.TotalWager != 50 ||
		req.TargetProfit != 0 || req.MinOdds != 3 || req.Label != "DAVO" {
		t.Errorf("cash request = %+v, want $50 place, highest odds first", req)
	}
}
