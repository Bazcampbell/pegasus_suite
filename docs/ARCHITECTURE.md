# Architecture

One repo, one module (`pegasus_suite`), one binary (`cmd/wagering`). No SDK, no
engine service, no logger service.

## Shape

```
cmd/wagering/main.go        wiring only: bucket → logger → kernel → apps → API
cmd/migrate-settings/       one-shot: Postgres user_settings rows → JSON documents
kernel/                     the host: runtime, process registry, control plane
engine/                     the betting engine: sessions, staking, placement
apps/<name>/                an application: its feeds, its logic, its settings
betting/                    betting.Client + betfair/ + betmatic/ (tote later)
clients/                    the client store: doc/ (JSON in a bucket), mem/ for tests
logger/                     stderr + in-memory ring; outputs plug in as Sinks
logger/telegram/            the Telegram Sink: queue, pacing, dedupe, cards
platform/                   auth (JWKS), store (file:// and s3:// buckets), util
```

No database. Settings are JSON documents in a bucket; logs live in memory
(and Telegram); auth stays with SESSION.

### Import rule

| package | imports | never imports |
|---|---|---|
| `betting`, `logger`, `platform` | stdlib + third party | anything else here |
| `clients` | nothing here | kernel, apps |
| `engine` | `betting`, `clients` (ProcessKey), `logger` | kernel, apps |
| `kernel` | `clients`, `engine`, `logger` | any `apps/*` |
| `apps/*` | `engine`, `clients` (types), `kernel` (interfaces only), `logger` | other apps, `clients/pg` |
| `cmd/wagering` | everything | — |

`main.go` is the only file that imports both the kernel and an application.

## The three interfaces

```go
// kernel/kernel.go
type App interface {
	Name() string
	Start(ctx context.Context, h Host) error                      // feeds, admin clients
	Stop()
	NewProcess(key clients.ProcessKey, settings json.RawMessage, h Host) (Process, error)
	ProcessSettings() Settings                                    // the type a process document decodes into
	AdminSettings() map[string]func() Settings                    // admin documents it owns, name → type
}

type Settings interface { Validate() error }                    // every settings type validates itself

type Process interface { Start(); Stop(); Running() bool; Close() }

type Host interface {                  // what the kernel hands down
	Settings(name string) (json.RawMessage, error)    // admin-level settings document
	Engine() *engine.Engine                           // the betting engine
}
```

## Settings

JSON documents in a bucket (`SETTINGS_URL`: `file:///path` for dev,
`s3://bucket/prefix` for real), read through `clients.Store`:

```
settings/apps/<name>.json                        admin-level, one per feed or account:
                                                 triples, tpd (Pegasus's); betfair, betmatic (shared)
settings/processes/<app>/<user_id>/<pid>.json    one process; the app decodes it
state/<app>/<user_id>/<pid>.json                 {"state": "running"} — written by the kernel
```

The kernel is the only writer. Users save their own process documents, admins
save anyone's plus the admin-level ones, all through the settings routes (below):
the kernel decodes the document into the type its owner names — an app's
`ProcessSettings()` or one of its `AdminSettings()`, or `engine.BetfairCredentials`
/ `engine.BetmaticCredentials` for the shared accounts — refusing unknown
fields, and runs that type's `Validate()`. Each admin document has exactly one
owner; a second claim panics at `Register`. Only then is it written. Checks that
depend on the running app (is this scope's feed enabled?) happen when the
process is built. A saved process that is loaded is rebuilt at once and left
running if it was; admin-level documents apply on the next runtime restart.
Settings and state are different objects so a save never races a state write.
Turn on bucket versioning and every settings change is an
audit trail with rollback. Secrets are plain inside the documents; bucket
encryption and IAM protect them. Each app documents its shape at the top of
its `settings` package. `cmd/migrate-settings` converts the old
`user_settings` rows once.

## Logs

`logger` keeps the last `LOG_RING_SIZE` entries in memory
and serves them at `GET /api/logs?level=&app=&process=&user=&since=&limit=`;
non-admins only see their own. Append is one slot write under a mutex, so the
bet path pays nothing it would notice. Bets never go in the ring. Telegram
gets `LOG_TG_LEVEL` and up, plus every bet. Nothing is written to disk or a
database.

Every line names a race the same way: `RaceDetails.Venue` is the canonical
Betmatic track name whichever feed or bookmaker produced it, set once per
packet in the feed handlers (`onTripleS`, `onTPD`) via `core.CanonicalVenue`.
`core.BetmaticNameForBetfair` maps a Betfair track for anything logging from
that side.

## Engine

`engine/` is one instance per runtime, shared by every process of every app.
It owns the bookmaker sessions, the staking arithmetic, request building and
placement. It never reads settings and has no opinion about which runner or
when.

| decides | owner |
|---|---|
| which runner, which side, now or not yet, confidence (unit) | strategy — pure, sees the race clock |
| dollars, MBL, acceptable odds, delay | scope settings — per process, per country/code |
| unit × stake → liability / target profit / limit price / ticks | engine |
| which session, label, place, (later) dedupe | engine |
| which bet *type* (Betmatic FIXED_PROFIT vs HIGH_ODDS_FIRST) | app's dispatch (thin) |

```go
acct, _ := h.Engine().Account(key, creds)          // NewProcess: open/join sessions once
eng.Place(engine.Order{Account, Event, Side, Runner, Unit, Stake, Label})   // per bet, own goroutine
```

The hot path (packet → fan-out → strategy → `Place`) does no lookups: account,
scope stake and venue names are resolved when the process is built, and the
Betfair book arrives on the Order from the background runner poller. The only
cost is the bookmaker round trip.

An application declares what it needs through `Host`. It never touches the
database, never holds a credential it wasn't handed, and never knows the kernel
exists beyond these types.

## Kernel

`kernel.Kernel` is what PEGASUS's `runtime.Supervisor` + `engine` registry and
DAVO's were, once:

- **Registry is flat**: `map[clients.ProcessKey]Process`, key `{App, UserID, ProcessID}`
  — the same identity as the `user_settings` primary key.
- **Boot starts nothing.** `main.go` builds the kernel and serves the API; the
  runtime starts only on `POST /api/system/start`, so a deploy never connects
  a feed or places a bet by itself.
- **Start** builds the engine (sessions), calls every app's `Start`, then
  restores processes from the store. One app failing to start is reported in
  `Status().Apps[name].Error` and does not stop the others. All of them failing
  fails the start.
- **Process state is persisted** as `state/<app>/<user>/<pid>.json` in the
  bucket. Starting the runtime after a container restart restores the set that
  was running.
- **Sessions** live in the engine (`engine/account.go`), keyed by
  provider+username and refcounted by holding process. Deleting one of two
  processes on the same Betmatic email does not log the other out.
- **Restart of a process** stops and closes the old one, releases its sessions,
  and builds a new one from current settings. Same-username re-login is
  accepted as the price of not tracking generations.

## Control plane

`kernel/api`. Routes are generic over the application in the path:

```
POST /api/{app}/add|start|stop|restart|delete?processId=   any user, own processes
GET  /api/{app}/status?processId=
GET  /api/{app}/processes                                    [{id, status}] the user's processes
GET  /api/{app}/settings?processId=                          the process's settings document
PUT  /api/{app}/settings?processId=                          validate → save → reload if loaded
DELETE /api/{app}/settings?processId=                        unload, forget state, delete document
                                                             (all of the above: admins may add &userId=)
GET  /api/betting/bookmakers
POST /api/system/start|stop|restart                          admin
GET  /api/system/status                                      admin; includes per-app detail
GET  /api/system/settings/{name}                             admin; triples, tpd, betfair, betmatic (defaults if unsaved)
PUT  /api/system/settings/{name}                             admin; validate → save
GET  /api/health
```

ADMIN's per-app base URLs become one host with `/api/pegasus/...`.

## Pegasus (`apps/pegasus`)

Same internals as before, minus everything the kernel now owns:

- `app.go` — `Start` = admin Betfair client + Triple-S + TPD (what `runtime/setup.go`
  did); `onTripleS` / `onTPD` = the fan-out, one per feed, each offering its own
  raw message type (what `engine.Handle` did); `NewProcess` = parse
  settings, validate scopes, claim sessions from `Accounts`, build a process.
- `settings/` — `ProcessSettings` and the parsers from `clients.Values`. This is
  the old `store` package with the SQL removed.
- `process/` — the old `tenant`. One inbox per feed (`triples.RaceMessage`,
  `tpd.Progress`), one strategy per feed, dispatch.
- `dispatch/` — maps the feed's venue to bookmaker names, attaches the Betfair
  book, builds an `engine.Order` with label `pegasus_<pid>`. ~40 lines.
- `strategy/` — one strategy per feed, no interface: `ForwardProgress` takes
  `triples.RaceMessage` (thoroughbred and harness), `TPDLeader` takes
  `tpd.Progress`. Each gets the raw message plus its `core.RaceRef`.
- `core/`, `tpd/`, `triples/` — `core.RaceRef` is a race's identity, derived by
  each feed once per message. `core.Side` and `core.BetmaticVenue` are aliases
  of the engine's types.

### Scope

A *scope* is one `country/code` key inside a process — `US/THOROUGHBRED`,
`AU/HARNESS`. It is the unit of money: each scope row (`meta2` in
`user_settings`) carries its own stake, MBL, min/max odds and delays. A
process bets a race iff it has an *active* scope (something staked) for that
race's country and code; that lookup is `Wants`, an atomic load and one map
read on the fan-out goroutine. The strategy follows from the feed — Triple-S
goes to `ForwardProgress`, TPD to `TPDLeader` — and keeps per-race state keyed
by the race, so two processes never share an "already bet" flag. Validated at
add time: the feed covering the country must be enabled and its strategy must
bet that racing code (`strategy.Covers`).

Adding a strategy: one file in `strategy/`, one case in `strategy.For`.

## Adding an application

1. `apps/<name>/app.go` implementing `kernel.App`. Own your feeds in `Start`.
2. Decode `clients.Values` into your own settings type in `NewProcess`.
3. One line in `main.go`: `k.Register(<name>.New())`.

That is the whole surface. The API, the registry, the sessions, the restore on
boot, and the logging come for free.

## Logging

`logger` is the old LOGGER service as a package, minus the Postgres sink.
`main.go` builds a `logger.Config` (stderr and ring levels, ring size) and,
separately, a `telegram.Config`. `telegram.New` checks the bot token; if it
fails, the logger runs without Telegram. Both go to
`logger.Init(cfg, sinks...)`. Any output is a `logger.Sink`
(`Enabled`/`Enqueue`/`Stop`); the core doesn't know about Telegram.

Every line becomes one `logger.Record`: the time, level, fallback app and user,
call site, and `Request`/`Response` snapshotted as JSON. The ring keeps it,
`GET /api/logs` serves it, and each Sink is handed it.

Telegram: bets go to `LOG_TG_BET_CHANNEL_ID`, and `LOG_TG_LEVEL` and up go to
`LOG_TG_CHANNEL_ID`. Each channel is paced to 18 messages a minute, a 429 is
waited out, and repeats within `LOG_TG_DEDUPE_MS` collapse into "+N more". It
uses the Bot API over plain HTTP, with no library.

The package-level `logger.Info/Warn/Error/Bet/Debug` are the one deliberate
global. Neither package reads the environment.

## Env

Every variable is required and read with `util.MustEnv` in the entry point's
`main.go` only; packages take their values as arguments. `.env.example` lists
them all.

```
SETTINGS_URL                 file:///var/lib/wagering or s3://bucket/prefix
ADMIN_USER_ID
WAGERING_PORT JWK_URL JWT_ISSUER             (SESSION sets no aud)
LOG_LEVEL LOG_RING_SIZE
LOG_TG_BOT_TOKEN LOG_TG_CHANNEL_ID LOG_TG_BET_CHANNEL_ID LOG_TG_QUEUE_SIZE LOG_TG_DEDUPE_MS
AWS_REGION + credentials via the SDK default chain when SETTINGS_URL is s3://
```

`docs/postman/wagering.postman_collection.json` covers every route.

## Status

- [x] kernel, accounts, API, client store (pg + mem)
- [x] pegasus ported; existing `tpd`/`triples`/`util` tests green
- [ ] davo — parked in `apps/_davo` (underscore = ignored by the Go toolchain)
      until ported; still references the ENGINE client
- [ ] ADMIN pointed at the new port and `/api/pegasus/...` paths
- [ ] tote provider under `betting/tote`
