# Roadmap

Where the suite is today, what's being asked for next, and how it should fit
together. This is a plan only. Each section ends with the decisions that are
still open.

---

## 1. Where things stand (2026-09-25)

**Shape.** One binary (`cmd/pegasus`) with these parts: a kernel (process
registry and control plane), one shared `engine` (sessions, staking,
placement), apps (`pegasus` running and `_davo` parked), `betting/{betfair,betmatic}`
provider clients, a `logger` with stderr, ring and Telegram sink, and settings
stored as JSON in a bucket (S3 or file).

**The build is broken right now.** A refactor is half done:

| break | where |
|---|---|
| `betting.Bet` no longer exists (renamed `BookmakerBet`?) | `betting/betfair/bet.go` |
| `PlaceBet` now returns `(id, err)`, but it returns `[]InstructionReport` as the id and drops `err` | `betting/betfair/betting.go:90` |
| `engine.Placer` still declares `PlaceBet(...) error` | `engine/account.go` |
| `engine.BetmaticVenue` is referenced by `apps/pegasus/core` but isn't defined | `engine/` |
| `betmatic.GetBet` is commented out | `betting/betmatic/bet.go` |

**What already fits the goals**
- The engine is the single place bets go through (`engine.Place(Order)`). That's the right seam for dedupe.
- Sessions are pooled and refcounted per bookmaker username.
- The logger is one package with typed `Log` and `BetLog`, and Telegram plugs in as a `Sink`.
- A canonical venue already exists: `core.CanonicalVenue` gives the Betmatic name.

**What gets in the way**
- **Sources do work the engine should do.** Pegasus's `dispatch` resolves the Betmatic venue and attaches the Betfair book before calling the engine. Every new source would have to copy that.
- **The venue tables live inside `apps/pegasus/core`** (the 906-line `mapping.go`). DAVO and the engine can't use them without importing an app.
- **Betfair prices are REST-polled** once a second per race (`runnerUpdateLoop`). That's slow and costs one request per race per second.
- **The bet path is slowed by logging.** `placeBetmatic` and `placeBetfair` log `Debug` synchronously, including a stderr write and JSON-marshalling `Request`, before the network call.
- **Log calls are verbose.** Each one is 5–7 lines of repeated `Application/UserID/ProcessID/RaceDetails`, and `main.go` and `kernel/api` call `slog` directly.
- **DAVO has its own Telegram client** (`go-telegram-bot-api`), separate from the logger's.
- **The docs are stale.** `ARCHITECTURE.md` still describes `cmd/wagering`, `clients/pg` and `cmd/migrate-settings`.
- **No persistent bet record, no dedupe, no results or PnL.**
- **Boot starts nothing:** the runtime waits for `POST /api/system/start`. That works against the uptime goal (see §9).

---

## 2. Requirements (as given)

1. **Engine dedupe.** The engine remembers recently placed bets so two sources can't bet the same runner. Sources and providers don't know about it. Keep entries **4 days**, because DAVO tips far ahead. Dedupe is **per user**, so the engine keeps one session per user ID.
2. **Consistent logging** to Telegram and stderr everywhere.
3. **Canonical track names = Betmatic names.** Every bet carries **race number + runner number** so bets can be reconciled across Betmatic, tote and Betfair.
4. **Betfair Exchange Stream.** `StartRunnerUpdates` maintains `marketID → selectionID → PriceOverview`, where the overview holds back and lay to n levels plus LTP. When the stream says a market is resolved or suspended, drop that market's selections.
5. **Internal bet ID** = process ID + event (Betmatic venue) + race no + runner no + date-time.
6. **Comments.** A function comment says exactly what the function does. Inline comments only where code is non-obvious.
7. **Maximum simplicity.** No superfluous code.
8. **Latency.** Time from receiving information to placing a bet is absolutely minimal. Records and logs may lag or drop.
9. **99.99 % uptime**, which allows about 52 minutes of downtime a year.
10. **Morning report.** Runs on start and then every morning. Takes the previous day's bets, pulls results from Betmatic and Betfair by their IDs, and stores them in S3. Reports turnover, profit, POT, and attempted vs accepted bets for the previous day, the past week and the past year.
11. **Adding a data source is simple.**

---

## 3. Target shape

```
cmd/pegasus/           wiring only
kernel/                registry, control plane, boot
engine/                the black box: users, dedupe, venues→provider ids, prices, placement, ledger
  engine.go            Engine, Place
  user.go              per-user session: dedupe set (+ bookmaker sessions it holds)
  ledger.go            async bet records → S3 (write-behind, lossy by design)
  place_betmatic.go
  place_betfair.go
  stake.go
racing/                shared vocabulary: Race, Venue (Betmatic name), venue tables, BetID
betting/betfair/       REST (catalogue, orders, cleared orders)
betting/betfair/stream/  Exchange Stream client + price cache
betting/betmatic/
report/                morning job: settle → daily summary → roll-ups → Telegram
logger/                one API; stderr becomes async like every other sink
apps/pegasus, apps/davo  sources: parse feed → canonical Race + runner → engine.Place
```

The one rule: **the app resolves *where* (it already needs the Betfair market
and Betmatic venue to get race information), and the engine does everything
else: dedupe, IDs, staking, placement, records and logs.** The engine does no
venue mapping.

One `Order` is one **bet request**: one runner with one or more provider
legs. Dedupe is per request, not per leg.

```go
type Order struct {
	Key    clients.ProcessKey // app, user, process
	Race   racing.Race        // date (AEST), venue (Betmatic name), race no. Dedupe key and log identity.
	Runner int
	Unit   float64
	Stake  Stake

	Betmatic *BetmaticLeg // venue name as Betmatic spells it; nil = no Betmatic leg
	Betfair  *BetfairLeg  // market ID, selection ID, back/lay; nil = no Betfair leg
}
```

Pegasus today sends a separate order per side from separate goroutines.
Those become one `Order` whose legs the engine fires in parallel.

---

## 4. Canonical races and bet IDs

- **Move the venue tables into `racing/`**, out of `apps/pegasus/core`: Triple-S→Betmatic, TPD→Betmatic, and Betmatic→Betfair. Each source keeps only its own "feed name → Betmatic name" table. The Betmatic→Betfair and Betmatic→tote joins belong to the engine side, so a source never touches them.
- **`racing.Race{Date, Country, Code, Venue, Number}`** is the identity used everywhere: logs, ledger and dedupe. `logger.RaceDetails` should take it directly.
- **Internal bet ID:** `<processID>:<VENUE>:R<race>:<runner>:<YYYYMMDDTHHMMSS>`, in UTC. The ID is generated in the engine at placement time.
- **Length problem.** Betfair `customerRef` and `customerOrderRef` allow at most 32 characters, and `customerStrategyRef` 15. The full ID won't fit, and truncating it (what happens today) can collide. Options:
  - (a) **Recommended.** Reconcile on the **provider ID returned by `PlaceBet`**: the Betfair `betId` and the Betmatic notification ID. The ledger row maps provider ID → internal ID. Refs are then only for humans, so send `customerStrategyRef = processID[:15]`.
  - (b) Send a 32-character hash of the internal ID as the ref and keep hash → ID in the ledger.

  Either way, `PlaceBet` returning the provider ID (the refactor already under way) is what makes reconciliation work.

---

## 5. Dedupe (engine, per user)

```go
type user struct {
	mu     sync.Mutex
	placed map[betKey]time.Time // expires 4 days after the race date
}

type betKey struct {
	Race   racing.Race // includes the date, so a Tuesday tip never collides with next Tuesday
	Runner int
}
```

**Rule (decided):** one bet request per runner per user, ever. The first
`Order` for a runner claims it and may carry every provider leg. Any later
`Order` for that runner and user is blocked, from any app or process. For
example, Pegasus sends Betfair + Betmatic for runner 4, then DAVO tips
runner 4 and is blocked. Different users are independent. If **every** leg
fails, the claim is released.

- **Hot path:** one mutex, one map lookup and insert *before* the network call (reserve-then-place). That costs well under a microsecond. Users are looked up by ID in a `sync.Map`, or better, the `*user` pointer is resolved once when the process is built and stored on the `Account`, keeping to "no lookups on the hot path".
- **If placement fails** (a transport error, or the bookmaker rejects the bet), release the reservation so another source can try. Keep it once the bookmaker has accepted.
- **Expiry:** a janitor goroutine prunes entries every hour. Because the key includes the race date, the TTL only exists to free memory. It never decides correctness.
- **Surviving restarts:** each reservation is also sent to the ledger (§7). On boot the engine reloads the last 4 days of ledger rows into `placed`. Accepted risk: a crash within the write-behind window (about 1 s) could forget a reservation. If that's not acceptable, add a synchronous S3 PUT for DAVO-type orders only, since those aren't latency-critical.
- **"Session per user"** means this `user` struct. It's also a natural place to hang per-user limits later, such as a daily max turnover.

---

## 6. Betfair Exchange Stream

`betting/betfair/stream`: a TLS connection to `stream-api.betfair.com:443` that exchanges CRLF-delimited JSON.

- **Messages:** `authentication` (app key plus the session token from the existing client), then `marketSubscription` with `fields: [EX_BEST_OFFERS, EX_LTP, EX_MARKET_DEF]`, `ladderLevels: n` (at most 10) and `conflateMs: 0`.
- **Subscribe by filter, once.** Use `eventTypeIds:[7(,4339)]`, `marketTypes:[WIN]` and `countryCodes` from settings, and skip per-race subscriptions entirely. `StartRunnerUpdates(ctx, filter)` starts the stream for the runtime. There's no start or stop per race and no `runnerCancels` map. That deletes `runnerUpdateLoop`, `updateRunners` and the pegasus `setBetfairPriceFeed` plumbing. The default limit is 200 markets per connection, which is plenty for WIN markets on one racing day.
- **Cache:**
  ```go
  type PriceOverview struct { Back, Lay []Level; LTP float64 } // Level{Price, Size}, best first
  map[marketID]*atomic.Pointer[map[selectionID]PriceOverview]
  ```
  The stream goroutine is the only writer. It applies each delta to a copy and swaps the pointer. Readers (`Place`) do one atomic load with no lock, so placement never waits behind a stream update.
- **Market status:** handle `marketDefinition.status`. See the open question below before choosing `SUSPENDED`.
- **Reliability:** reconnect with backoff and resume from `initialClk`/`clk` so no deltas are lost. Treat a stream with no message for 2 × `heartbeatMs` as dead and reconnect. Surface stream status in `/api/system/status`.
- **Split of work:** the catalogue (venue, race and runner number → marketID and selectionID) stays REST with an hourly refresh, because the stream carries neither track names nor saddle-cloth numbers. The stream provides prices only.
- **Location:** the stream and cache live in the engine and run on the admin Betfair account. Sources no longer attach books to orders.

---

## 7. Ledger and the morning report

**Ledger (write-behind, lossy by design).** `engine.Place` hands a row to a
buffered channel with a non-blocking send, so a full buffer drops the row
instead of stalling the bet. A single writer batches rows to S3:

```
bets/<YYYY-MM-DD>/<userID>.jsonl        one line per attempt: internal id, provider, provider id,
                                        race, runner, side, requested, accepted?, error, ts
```

S3 has no append, so the writer keeps the day's file in memory and re-PUTs
it every N seconds. That's one small object per user per day.

**Morning job (`report/`).** A `time.Timer` fires on start and then daily at a
configured local time (for example 09:00 Australia/Sydney, after US racing
settles). It's **idempotent and self-healing**: it works out which days
since the last `summary/` object have no summary and fills them in order, so
a skipped morning or a restart catches up.

1. **Settle.** Pull the providers' own lists of bets for the day as the source of truth: Betfair `listClearedOrders` by date range, and Betmatic `GetNotifications` by date, then `GetNotificationBets` for each one. Join those to the ledger on provider ID to attach internal ID, process and race. This way PnL is exact even when ledger rows were dropped, and dropped rows only lose the "which process" attribution.
2. **Write** `results/<date>.json` (per bet) and `summary/daily/<date>.json`, per user and in total:
   - turnover
   - profit
   - POT (profit ÷ turnover)
   - attempted vs accepted count
   - pending count, if anything is still unsettled
3. **Roll up.** Week and year come from summing the last 7 and 365 small daily summaries, which is about 365 tiny GETs, or from one `summary/<year>.json` accumulator updated in place. Summing is simpler and can't drift, so start there.
4. **Send** one Telegram card: yesterday, the last 7 days and year-to-date.

Bets still pending at run time (for example late US races or Betfair
settlement lag) are flagged. The next morning's run re-settles any day marked
partial.

---

## 8. Logging

Keep the one `logger` package, but change the API and the hot-path cost.

- **One entry point per scope.** `log := logger.For(key)` is built once per process and carries app, user and process. Calls become `log.Race(r).Info("betmatic requested", "target", 12.5)`, one line each, and all identity fields are always present.
- **stderr becomes an async sink** like Telegram: a buffered channel, dropping on overflow. JSON marshalling of `Request`/`Response` moves to the sink goroutine. `emit` on the bet path then only does a non-blocking channel send.
- **No log calls before the network call** in `place*`. Log after the bookmaker returns.
- **Nothing else writes output.** `main.go` and `kernel/api` use `logger`. DAVO's Telegram client is only for *reading* tip channels, and outbound messages go through the logger's Telegram sink. Drop `go-telegram-bot-api` if DAVO's reader can use the same plain-HTTP client.
- **Every bet log carries** the internal bet ID, Betmatic venue, race number and runner number.

---

## 9. Uptime (99.99 %)

- **Resume on boot.** Replace "boot starts nothing" with an explicit persisted desired state: `system.json {"running": true}`. A container restart then resumes betting without anyone pressing start. A deploy that shouldn't bet sets it to false first.
- **Recover panics.** Every goroutine boundary (feed callbacks, `go Place`, stream reader, ledger writer, report job) runs through one `safeGo` that recovers panics, logs them at Error and keeps the process alive. Today a panic in any `go p.dispatcher.Place` kills the binary.
- **Reconnect everything** with capped backoff: the Betfair stream, Triple-S MQTT, TPD UDP and the Betmatic session. Report liveness in `/api/health` so the orchestrator restarts only a truly wedged container.
- **Graceful handover on deploy.** SIGTERM stops taking new orders, drains in-flight `Place` calls and flushes the ledger, with the existing 15 s timeout.
- **Keep bookmaker token refresh** as it is, but alert (Telegram Error) after the second consecutive refresh failure, not the fifth.

---

## 10. Adding a data source

With the above in place, a new source is:

1. `apps/<name>/app.go` implementing `kernel.App`, which owns its feed.
2. A table mapping the feed's venue names to Betmatic names, in `racing/`, if they differ.
3. Decide the runner, then call `eng.Place(engine.Order{Key, Race, Runner, Side, Unit, Stake})`.
4. One line in `main.go`.

Sessions, dedupe, prices, IDs, the ledger, logging and PnL all come for free. DAVO is the first test of this.

---

## 11. Suggested order

1. **Fix the build.** Settle the `BookmakerBet` refactor so that `PlaceBet` returns the provider ID, and define `engine.BetmaticVenue` or move it to `racing`.
2. Add `racing/` (Race, venue tables, BetID) and move the mapping out of pegasus.
3. Add the logger API (`For`, `Race`) and the async stderr sink, and convert call sites.
4. Engine: `Order` → canonical, per-user dedupe, and `safeGo`.
5. Betfair stream and cache, then delete the REST polling.
6. Ledger (write-behind) and dedupe reload on boot.
7. Morning report.
8. Port DAVO onto the new `Order`.
9. Resume on boot (persisted desired state); rewrite `ARCHITECTURE.md` to match.

---

## 12. Decisions (2026-09-25)

- **No venue mapping in the engine.** Apps pass the Betfair market and selection IDs and the Betmatic venue on the order.
- **Logging is 100 % asynchronous** on every output. Dropped lines are acceptable.
- **Stream:** a market's prices are dropped on `CLOSED` only.
- **Dedupe:** one request per runner per user, across all apps. A request may carry several provider legs. The claim is freed if the whole request fails.
- **Day boundary:** AEST (`Australia/Brisbane`, no DST drift).
- **Tote:** deferred.
- **DAVO:** bets are placed immediately. No scheduled-order queue.

## 13. Superseded open questions

1. **Suspended vs closed.** Horse racing markets suspend at the jump and also briefly for scratchings, then reopen. If the cache deletes on `SUSPENDED`, the book is lost until the next full image. Should it delete on `CLOSED` only and mark `SUSPENDED` as not bettable? Or is SUSPENDED the intended signal because you never bet in-play?
2. **What counts as a duplicate?** Is it the same user, race and runner across *all* providers, or per side? For example, is a Betmatic win bet plus a Betfair back on the same runner a duplicate? Is a back plus a lay a duplicate?
3. **Placement fails.** Should the dedupe slot be released so another source can retry (recommended), or held?
4. **Bet ID versus refs.** Reconcile on the provider-returned ID (recommended), or on a hashed ref?
5. **Report timing.** Which timezone defines "the day", and what time should the job run?
6. **Report audience.** Is the report per user, total only, or both? Does it go to the logs channel, the bets channel or a new one?
7. **Tote.** Is it its own provider (`betting/tote`), or reached through Betmatic's tote markets? This affects whether reconciliation has two sources or three.
8. **DAVO lead time.** DAVO tips days out. Should it place immediately (Betmatic notification), or park until the market opens (Betfair)? If it parks, the engine needs a small scheduled-order queue.
