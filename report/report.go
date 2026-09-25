// report/report.go
//
// The morning report. On start and every morning it settles each of the last
// seven days that has no settled summary yet, stores it in the bucket, and
// sends each user yesterday, the last seven days and the month to date.
//
//	reports/daily/<YYYY-MM-DD>.json    user ID → Totals for that AEST day

package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"pegasus_suite/betting"
	"pegasus_suite/clients"
	"pegasus_suite/engine"
	"pegasus_suite/logger"
	"pegasus_suite/platform/store"
)

const (
	app        = "report"
	runAtHour  = 9
	retryAfter = 10 * time.Minute
	lookback   = 7
)

// aest is Australian Eastern Standard Time, fixed so it needs no tz database.
var aest = time.FixedZone("AEST", 10*60*60)

// Runtime is what the report needs from the kernel.
type Runtime interface {
	Engine() *engine.Engine // nil while the runtime is stopped
	Apps() []string
}

type Totals struct {
	Turnover  float64 `json:"turnover"`
	Profit    float64 `json:"profit"`
	Attempted int     `json:"attempted"`
	Accepted  int     `json:"accepted"`
	Pending   int     `json:"pending"`
}

type Reporter struct {
	bucket  store.Bucket
	clients clients.Store
	runtime Runtime
}

// Start runs the report now and then every morning at runAtHour AEST until ctx ends,
// retrying every retryAfter while a run fails.
func Start(ctx context.Context, bucket store.Bucket, cs clients.Store, rt Runtime) {
	r := &Reporter{bucket: bucket, clients: cs, runtime: rt}
	go func() {
		for {
			wait := untilNextRun(time.Now())
			if err := r.run(ctx, time.Now()); err != nil {
				logger.Warn(logger.Log{App: app, Message: fmt.Sprintf("report failed, retrying in %v error=%v", retryAfter, err)})
				wait = retryAfter
			}
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return
			}
		}
	}()
}

func untilNextRun(now time.Time) time.Duration {
	local := now.In(aest)
	next := time.Date(local.Year(), local.Month(), local.Day(), runAtHour, 0, 0, 0, aest)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Sub(local)
}

// run settles every recent day without a settled summary and sends yesterday's report the first time it is settled.
func (r *Reporter) run(ctx context.Context, now time.Time) error {
	eng := r.runtime.Engine()
	if eng == nil {
		return errors.New("runtime not running")
	}

	today := day(now)
	yesterday := today.AddDate(0, 0, -1)
	for d := today.AddDate(0, 0, -lookback); d.Before(today); d = d.AddDate(0, 0, 1) {
		stored, found, err := r.load(ctx, d)
		if err != nil {
			return err
		}
		if found && !pending(stored) {
			continue
		}
		totals, err := r.settle(eng, d)
		if err != nil {
			return err
		}
		if err := r.save(ctx, d, totals); err != nil {
			return err
		}
		if d.Equal(yesterday) && !found {
			r.send(ctx, yesterday)
		}
	}
	return nil
}

// settle returns each user's totals for day d from the providers' own records.
func (r *Reporter) settle(eng *engine.Engine, d time.Time) (map[string]Totals, error) {
	accounts, err := r.accounts()
	if err != nil {
		return nil, err
	}
	out := make(map[string]Totals)

	var admin engine.BetmaticCredentials
	doc, err := r.clients.AppSettings("betmatic")
	if err == nil {
		err = unmarshal(doc, &admin)
	}
	if err != nil {
		return nil, fmt.Errorf("betmatic admin: %w", err)
	}
	if admin.Validate() == nil {
		bets, err := eng.BetmaticBets(admin, d.Format(time.DateOnly))
		if err != nil {
			return nil, err
		}
		for _, b := range bets {
			if user, ok := accounts.botUsers[b.Bot]; ok {
				out[user] = out[user].add(b)
			}
		}
	}

	for user, creds := range accounts.betfair {
		for _, c := range creds {
			bets, err := eng.BetfairBets(c, d, d.AddDate(0, 0, 1))
			if err != nil {
				return nil, fmt.Errorf("betfair %s: %w", c.Username, err)
			}
			for _, b := range bets {
				out[user] = out[user].add(b)
			}
		}
	}
	return out, nil
}

func (t Totals) add(b betting.Bet) Totals {
	t.Attempted++
	switch b.Status {
	case betting.BetLapsed:
		return t
	case betting.BetPending:
		t.Pending++
	}
	t.Accepted++
	t.Turnover += b.Liability
	t.Profit += b.Profit
	return t
}

func (t Totals) plus(o Totals) Totals {
	return Totals{
		Turnover:  t.Turnover + o.Turnover,
		Profit:    t.Profit + o.Profit,
		Attempted: t.Attempted + o.Attempted,
		Accepted:  t.Accepted + o.Accepted,
		Pending:   t.Pending + o.Pending,
	}
}

type accounts struct {
	botUsers map[string]string                      // Betmatic bot ID → user ID
	betfair  map[string][]engine.BetfairCredentials // user ID → distinct Betfair accounts
}

// accounts reads every process document of every app for the Betmatic bots and Betfair accounts it names.
func (r *Reporter) accounts() (accounts, error) {
	out := accounts{botUsers: map[string]string{}, betfair: map[string][]engine.BetfairCredentials{}}
	seen := map[string]bool{}
	for _, name := range r.runtime.Apps() {
		refs, err := r.clients.Processes(name)
		if err != nil {
			return out, err
		}
		for _, ref := range refs {
			var doc struct {
				Betmatic engine.BetmaticCredentials `json:"betmatic"`
				Betfair  engine.BetfairCredentials  `json:"betfair"`
			}
			raw, err := r.clients.Process(ref.Key)
			if err != nil || unmarshal(raw, &doc) != nil {
				continue
			}
			user := ref.Key.UserID
			if doc.Betmatic.BotID != "" {
				out.botUsers[doc.Betmatic.BotID] = user
			}
			if doc.Betfair.Validate() == nil && !seen[user+"/"+strings.ToLower(doc.Betfair.Username)] {
				seen[user+"/"+strings.ToLower(doc.Betfair.Username)] = true
				out.betfair[user] = append(out.betfair[user], doc.Betfair)
			}
		}
	}
	return out, nil
}

// unmarshal decodes doc into v, leaving v alone when doc is empty.
func unmarshal(doc json.RawMessage, v any) error {
	if len(doc) == 0 {
		return nil
	}
	return json.Unmarshal(doc, v)
}

func pending(totals map[string]Totals) bool {
	for _, t := range totals {
		if t.Pending > 0 {
			return true
		}
	}
	return false
}

// day returns the AEST calendar day containing t, as midnight AEST.
func day(t time.Time) time.Time {
	local := t.In(aest)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, aest)
}

func dailyKey(d time.Time) string { return "reports/daily/" + d.Format(time.DateOnly) + ".json" }

func (r *Reporter) load(ctx context.Context, d time.Time) (map[string]Totals, bool, error) {
	body, err := r.bucket.Get(ctx, dailyKey(d))
	if errors.Is(err, store.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var totals map[string]Totals
	if err := json.Unmarshal(body, &totals); err != nil {
		return nil, false, err
	}
	return totals, true, nil
}

func (r *Reporter) save(ctx context.Context, d time.Time, totals map[string]Totals) error {
	body, err := json.Marshal(totals)
	if err != nil {
		return err
	}
	return r.bucket.Put(ctx, dailyKey(d), body)
}
