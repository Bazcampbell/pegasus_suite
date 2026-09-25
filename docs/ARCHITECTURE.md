# Architecture

One repo, one module (`pegasus_suite`), one binary (`cmd/pegasus`).

```
cmd/pegasus/            wiring only: bucket → logger → kernel → apps → report → API
kernel/                 runtime, process registry, control plane (kernel/api)
engine/                 the black box: sessions, dedupe, prices, placement, bet reads
apps/pegasus/           Triple-S GPS feed → forward-progress strategy → bets
apps/davo/              tipster Telegram channel → tips → Betmatic bets
betting/betfair/        catalogue, Exchange Stream prices, BSP orders, cleared orders
betting/betmatic/       notifications, events, bookie control
report/                 the morning PnL report
clients/                settings store: doc/ (JSON in a bucket), mem/ for tests
logger/                 async logging: stderr, ring, sinks (logger/telegram)
platform/               auth (JWKS), store (file:// and s3:// buckets), util
```

## The rule

An app decides *what* to bet and resolves *where*: the Betmatic venue name,
the Betfair market and selection. It hands the engine an `engine.Order` and
knows nothing else. The engine owns dedupe, bet IDs, sizing, the request,
the bookmaker call, and the bet log. It does no venue mapping.

```go
eng.Place(engine.Order{
	Account:     account,                   // built once per process by eng.Account
	Race:        engine.Race{Date, Venue, Metro, Code, Number, MarketID},
	Runner:      4,
	SelectionID: 123456,                    // Betfair; 0 when none
	Side:        engine.BetfairBack,        // BetmaticWin, BetmaticPlace, BetfairBack, BetfairLay
	Unit:        0.8,
	Stake:       scope.Stake,
})
```

`Place` blocks for the bookmaker round trip, so callers run it on its own
goroutine. The bet path takes no locks beyond the user's claim map and does
no lookups; logging from it is a non-blocking channel send.

## Engine

- **Sessions** are pooled by username and refcounted by process
  (`join`/`leave`), so one login serves every process on the account, and a
  report or app reading through `eng.Betmatic`/`BetfairBets` never logs a
  live session out.
- **Bet ID** is `VENUE:R<race>:<runner>:<YYYYMMDD>` (Betmatic venue, spaces
  removed, race date in AEST). It is the runner's identity across providers.
  Betfair gets it as the order ref (last 32 characters) and the app name as
  the strategy ref; Betmatic gets the app name as the label.
- **Dedupe** is per user. A runner belongs to the first app that bets it;
  every process of that app may bet it once per provider, and every other app
  is refused (DEBUG log). A bet that fails frees its claim. Claims live in
  memory only: they clear when the stream reports the market `CLOSED`, or
  after 96 hours for bets with no Betfair market (DAVO). A restart forgets
  them.
- **Prices** come from the admin Betfair account (`settings/apps/betfair.json`),
  started by the kernel with `eng.StartBetfair`. The catalogue refreshes
  hourly (AU, 12 hours ahead); the Exchange Stream subscribes to AU
  thoroughbred WIN markets starting within the next four hours (and up to an
  hour past) and keeps three levels a side plus LTP per runner. The stream
  goroutine is the only writer; `Place` reads a per-market snapshot with no
  lock. Reconnects back off from 1s to 30s and resume from the last clock.

## Apps

```go
type App interface {
	Name() string
	Start(ctx context.Context, h Host) error      // feeds and admin clients
	Stop()
	Status() any
	NewProcess(key clients.ProcessKey, settings json.RawMessage, h Host) (Process, error)
	ProcessSettings() Settings
	AdminSettings() map[string]func() Settings    // admin documents it owns
}
```

**Pegasus.** Triple-S (MQTT) messages fan out to each process with an active
`AU/THOROUGHBRED` or `AU/HARNESS` scope. The forward-progress strategy picks
the runner that has advanced furthest (Betmatic win, Betfair back) and least
(Betfair lay) after each provider's delay; `dispatch` resolves the Betmatic
venue and Betfair IDs and places. Harness races have no streamed prices, so
their Betfair legs are skipped.

**DAVO.** Channel posts are parsed strictly and matched against Betmatic's
upcoming events; anything that reads like a bet but does not parse goes to
the model (`claude-opus-5`) with the real field. Photos are transcribed
first. Each running process stakes cash, highest odds first, at the rated
price less `min_odds_threshold`, sized so one unit risks `target_liability`.
Posts that are not bets arm the bookies.

**Adding an app:** `apps/<name>/app.go` implementing `kernel.App`, resolve
your races to Betmatic names (and Betfair IDs if you bet there), build
`engine.Order`s, and add one line to `main.go`.

## Kernel

Boot builds the kernel and serves the API, then resumes the runtime if it
was running when the binary last exited (`state/runtime.json`). Start builds
the engine, starts admin Betfair and every app (one failing doesn't stop the
others), and restores each process's last state. The API's start, stop and
restart record the runtime state; process exit (`Shutdown`) leaves it alone.

## Report

On start and at 09:00 AEST, for each of the last 7 days without a settled
summary: Betfair cleared orders per user account and the admin Betmatic
account's notifications (split to users by bot ID) are totalled per user and
stored at `reports/daily/<date>.json`. The first time yesterday settles, each
user gets a card in the bets channel: yesterday, last 7 days, month to date —
turnover (liability), profit, POT, accepted/attempted, pending.

## Logging

`logger.Debug/Info/Warn/Error(logger.Log{...})` and `logger.Bet(logger.BetLog{...})`.
A call captures its call site and queues the line; one goroutine writes it to
stderr, the ring (`GET /api/logs`) and each sink. A full queue drops the line.
Telegram gets `LOG_TG_LEVEL` and up in the log channel and every bet in the
bets channel. Every race is named by its Betmatic venue with race and runner
numbers. `defer logger.Recover(app)` guards every goroutine boundary.

## Settings

JSON documents in the bucket (`SETTINGS_URL`), written only by the kernel
after the owning type's `Validate`:

```
settings/apps/<name>.json                       admin: triples, davo, betfair, betmatic
settings/processes/<app>/<user>/<pid>.json      one process
state/<app>/<user>/<pid>.json                   process state
state/runtime.json                              runtime state
reports/daily/<date>.json                       report totals
```

## Env

Read only in `main.go`; see `.env.example`.
