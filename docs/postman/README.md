# Testing the wagering API locally

Two files in this folder:

| File | What it is |
|---|---|
| `wagering.postman_collection.json` | Every route, with example bodies and a status check on each request |
| `local.postman_environment.json` | Local URLs and your logins (`Wagering (local)`) |

Import both into Postman (**Import** → drop the two files), then pick **Wagering (local)** in the environment dropdown, top right.

## 1. Run SESSION

The wagering service does not issue tokens; it checks SESSION's. Run SESSION the usual way (from the suite root, `./deploy.sh DEV`, or `go run .` inside `SESSION/`). It must be reachable at `http://localhost:8000` (the environment's `sessionUrl`).

You need two SESSION users:

- one with the **`pegasus`** role, for normal process requests
- one with the **`admin`** role, for runtime control and the admin settings documents

(One user with both roles works for both.) Roles are set on ADMIN's Users page.

## 2. Run the wagering service

```bash
cp .env.example .env      # once
```

Edit `.env`:

```bash
# a local folder: the settings and state documents land here, easy to inspect
SETTINGS_URL=file:///C:/wagering-local        # Windows; file:///tmp/wagering on Linux/macOS
WAGERING_PORT=8080
JWK_URL=http://localhost:8000/.well-known/jwks.json
JWT_ISSUER=bazbet-session                      # SESSION's "issuer" setting; this is its default
```

Leave the `LOG_TG_*` values as the mock ones in `.env.example` unless you want local test logs in the real Telegram channels. With a mock token it prints `telegram logging unavailable` once and carries on.

```bash
go run ./cmd/pegasus
```

Boot starts nothing but the API. The only log line should be `api server starting port=8080`. The runtime starts when you ask it to (step 4).

## 3. Log in

In the environment, fill in `username` / `password` (pegasus user) and `adminUsername` / `adminPassword` (admin user).

Send both requests in **Auth (SESSION)**. They store `token` and `adminToken` for everything else. SESSION tokens are short lived: when requests start returning **401**, send the logins again.

> Don't add `token` or `adminToken` to the environment. The login scripts save them as collection variables, and an environment variable of the same name would override them with a stale value.

## 4. First run, in this order

1. **Health**: 200 `{"status":"online"}`.
2. **Admin settings**
   - **Save betfair** with a real account (username, password, app key, and the combined cert + key PEM). Pegasus will not start without it.
   - **Save triples** with real Triple-S details, or set `"enabled": false`.
   - **Save tpd**. It's off by default; turning it on needs `licence_key`.
3. **System (admin)** → **Restart runtime**, then **System status**. Each feed shows `running` or its `error`. A start failure is in `apps.pegasus.error`.
4. **Process settings** → **Save settings** (204), then **List processes**, which shows `local-test-1` as `not-added`.
5. **Process lifecycle** → **Add**, then **Start**. **List processes** now shows `active`.
6. **Process settings** → **Delete process and settings** when you're done.

Change `processId` in the environment to work on a different process.

## What to expect

| Code | Means |
|---|---|
| 200 / 204 | Done |
| 400 | Bad input or invalid settings. The body is the reason, e.g. `invalid settings: tpd licence key not set` |
| 401 | No token, or it expired. Log in again |
| 403 | Not an admin, or another user's process |
| 404 | Unknown app, settings document or process (e.g. **Add** before **Save settings**) |
| 409 | Runtime state conflict (e.g. process ops while the runtime is stopped), or **saved but the running process did not reload** (the body says why) |
| 500 | Start or restart failed. **System status** says why |

The requests named `(invalid -> 400)`, `(unknown field -> 400)` and `-> 404` are meant to fail. Their checks pass when they do.

Settings saved from the **Admin settings** folder apply the next time the runtime starts. Process settings apply immediately: a loaded process is rebuilt, and left running if it was.

## Acting for another user (admin)

Every process request has a `userId` query param, switched off by default. To work on someone else's processes:

1. Set `userId` in the environment to their SESSION user id.
2. On the request's **Params** tab, tick `userId`.
3. On the **Authorization** tab, set the token to `{{adminToken}}`.

Without the admin token, naming another user is **403**.

## Looking at what was saved

With a `file://` bucket the documents are plain files:

```
C:\wagering-local\
  settings\apps\betfair.json
  settings\apps\triples.json
  settings\processes\pegasus\<your user id>\local-test-1.json
  state\pegasus\<your user id>\local-test-1.json
```

Delete the folder to start from nothing.

## Going through ADMIN instead

To test the same path the browser uses, run ADMIN's dev server (`npm run dev` in `ADMIN/`, port 3001) and set `baseUrl` to `http://localhost:3001/wagering-api`.

## From the command line

The collection runs under [newman](https://github.com/postmanlabs/newman). Pass tokens in directly to skip the login step:

```bash
npx newman run docs/postman/wagering.postman_collection.json \
  -e docs/postman/local.postman_environment.json \
  --env-var token=<user token> --env-var adminToken=<admin token> \
  --folder "Admin settings" --folder "Process settings"
```
