# Architecture

One repo, one module (`racing_wagering`), one binary (`cmd/wagering`). No SDK, no
engine service, no logger service.

## Shape

```
cmd/wagering/main.go        wiring only: bucket → logger → kernel → apps → API
cmd/migrate-settings/       one-shot: Postgres user_settings rows → JSON documents
kernel/                     the host: runtime, process registry, control plane
engine/                     the betting engine: sessions, staking, placement, results
apps/<name>/                an application: its feeds, its logic, its settings
betting/                    betting.Client + betfair/ + betmatic/ (tote later)
clients/                    the client store: doc/ (JSON in a bucket), mem/ for tests
logger/                     stderr + in-memory ring + Telegram, in-process
platform/                   auth (JWKS), blob (file:// and s3:// buckets), util
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
	NewProcess(key clients.ProcessKey, settings clients.Values, h Host) (Process, error)
}

type Process interface { Start(); Stop(); Running() bool; Close() }

type Host interface {                  // what the kernel hands down
	App(scope string) (json.RawMessage, error)        // admin-level settings document
	Engine() *engine.Engine                           // the betting engine
}
```

## Settings

JSON documents in a bucket (`SETTINGS_URL`: `file:///path` for dev,
`s3://bucket/prefix` for real), read through `clients.Store`:

```
settings/apps/<scope>.json                       admin-level: pegasus, betfair, betmatic, davo
settings/processes/<app>/<user_id>/<pid>.json    one process; the app decodes it
state/<app>/<user_id>/<pid>.json                 {"state": "running"} — written by the kernel
```

Settings and state are different objects so ADMIN and the kernel never write
the same document. Turn on bucket versioning and every settings change is an
audit trail with rollback. Secrets are plain inside the documents; bucket
encryption and IAM protect them. Each app documents its shape at the top of
its `settings` package. `cmd/migrate-settings` converts the old
`user_settings` rows once.

## Logs

`logger` keeps the last `LOG_RING_SIZE` (default 10 000) entries in memory
and serves them at `GET /api/logs?level=&app=&process=&user=&since=&limit=`;
non-admins only see their own. Append is one slot write under a mutex, so the
bet path pays nothing it would notice. Telegram gets Warn/Error/Bet as before.
Nothing is written to disk or a database.

Every line names a race the same way: `RaceDetails.Venue` is the canonical
Betmatic track name whichever feed or bookmaker produced it, set once per
packet in `pegasus.Handle` via `core.CanonicalVenue`.
`core.BetmaticNameForBetfair` maps a Betfair track for anything logging from
that side.

## Engine

`engine/` is one instance per runtime, shared by every process of every app.
It owns the bookmaker sessions, the staking arithmetic, request building,
placement and the Betmatic results poller. It never reads settings and has no
opinion about which runner or when.

| decides | owner |
|---|---|
| which runner, which side, now or not yet, confidence (unit) | strategy — pure, sees the race clock |
| dollars, MBL, acceptable odds, delay | scope settings — per process, per country/code |
| unit × stake → liability / target profit / limit price / ticks | engine |
| which session, label, place, watch result, (later) dedupe | engine |
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
- **Start** builds `Accounts` and `Results`, calls every app's `Start`, then
  restores processes from the store. One app failing to start is reported in
  `Status().Apps[name].Error` and does not stop the others. All of them failing
  fails the start.
- **Process state is persisted** as `state/<app>/<user>/<pid>.json` in the
  bucket. A container restart restores the set that was running. This is the
  fix for "boots with zero processes".
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
GET  /api/betting/bookmakers
POST /api/system/start|stop|restart                          admin
GET  /api/system/status                                      admin; includes per-app detail
GET  /api/health
```

ADMIN's per-app base URLs become one host with `/api/pegasus/...`.

## Pegasus (`apps/pegasus`)

Same internals as before, minus everything the kernel now owns:

- `app.go` — `Start` = admin Betfair client + Triple-S + TPD (what `runtime/setup.go`
  did); `Handle` = the fan-out (what `engine.Handle` did); `NewProcess` = parse
  settings, validate scopes, claim sessions from `Accounts`, build a process.
- `settings/` — `ProcessSettings` and the parsers from `clients.Values`. This is
  the old `store` package with the SQL removed.
- `process/` — the old `tenant`. Inbox, per-scope strategy, dispatch.
- `dispatch/` — maps the feed's venue to bookmaker names, attaches the Betfair
  book, builds an `engine.Order` with label `pegasus_<pid>_<code>`. ~40 lines.
- `strategy/`, `core/`, `tpd/`, `triples/` — untouched. `core.Side` and
  `core.BetmaticVenue` are aliases of the engine's types.

### Scope

A *scope* is one `country/code` key inside a process — `US/THOROUGHBRED`,
`AU/HARNESS`. It is the unit of money: each scope row (`meta2` in
`user_settings`) carries its own stake, MBL, min/max odds and delays. A
process bets a race iff it has an *active* scope (something staked) for that
race's country and code; that lookup is `WantsMessage`, one map read on the
fan-out goroutine. The strategy is chosen by `(feed, code)` — the feed follows
from the country — and each scope gets its own strategy instance with its own
per-race state, so two scopes never share an "already bet" flag. Validated at
add time: the feed covering the country must be enabled and a strategy must
exist for the pairing.

Adding a strategy: one file in `strategy/`, one case in `strategy.For`.

## Adding an application

1. `apps/<name>/app.go` implementing `kernel.App`. Own your feeds in `Start`.
2. Decode `clients.Values` into your own settings type in `NewProcess`.
3. One line in `main.go`: `k.Register(<name>.New())`.

That is the whole surface. The API, the registry, the sessions, the restore on
boot, and the logging come for free.

## Logging

`logger` is the old LOGGER service as a package, minus the Postgres sink.
`logger.Init` in `main.go` with (optionally) Telegram from env —
`LOG_TG_BOT_TOKEN`, `LOG_TG_CHANNEL_ID`, and per-level
`LOG_TG_{INFO,WARN,ERROR,BET}_CHANNEL_ID`. The package-level
`logger.Info/Warn/Error/Bet/Debug` are the one deliberate global. Level from
`WAGERING_LOG_LEVEL` or `LOG_LEVEL`.

## Env

```
SETTINGS_URL                 file:///var/lib/wagering or s3://bucket/prefix
ADMIN_USER_ID
WAGERING_PORT JWK_URL [JWT_ISSUER JWT_AUDIENCE]
[LOG_LEVEL] [LOG_RING_SIZE] [LOG_TG_BOT_TOKEN LOG_TG_CHANNEL_ID ...]
AWS_REGION + credentials via the default chain when SETTINGS_URL is s3://
```

## Status

- [x] kernel, accounts, API, client store (pg + mem)
- [x] pegasus ported; existing `tpd`/`triples`/`util` tests green
- [ ] davo — parked in `apps/_davo` (underscore = ignored by the Go toolchain)
      until ported; still references the ENGINE client
- [ ] ADMIN pointed at the new port and `/api/pegasus/...` paths
- [ ] tote provider under `betting/tote`
