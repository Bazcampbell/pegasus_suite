// cmd/migrate-settings/main.go
//
// One-shot: read today's user_settings rows out of Postgres, decrypt the
// secrets, and write the JSON documents the kernel now reads. Run it once per
// environment, check the output, delete the table when you're sure.
//
//	DB_HOST=… DB_PORT=… DB_USER=… DB_PASSWORD=… DB_DATABASE=… \
//	ENCRYPTION_KEY=… ADMIN_USER_ID=… SETTINGS_URL=file:///tmp/settings \
//	go run ./cmd/migrate-settings [-dry-run]

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"

	"racing_wagering/clients"
	"racing_wagering/clients/doc"
	"racing_wagering/platform/blob"
	"racing_wagering/platform/util"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type row struct {
	app, userID, key, value, meta1, meta2 string
}

// encrypted names the keys stored AES-GCM'd in the old table.
var encrypted = map[string]bool{
	"betmatic_password": true, "betfair_password": true, "betfair_app_key": true, "betfair_pem_key": true,
	"secret_access_key": true, "tpd_licence_key": true, "telegram_bot_token_key": true, "anthropic_key": true,
}

func main() {
	_ = godotenv.Load()
	dryRun := flag.Bool("dry-run", false, "print the documents instead of writing them")
	flag.Parse()

	ctx := context.Background()
	admin := util.MustEnv("ADMIN_USER_ID")

	dec, err := util.NewDecrypter(util.MustEnv("ENCRYPTION_KEY"))
	if err != nil {
		log.Fatal(err)
	}

	rows, err := load(ctx, dec)
	if err != nil {
		log.Fatal(err)
	}

	docs := convert(rows, admin)

	var bucket blob.Bucket
	if !*dryRun {
		if bucket, err = blob.Open(ctx, util.MustEnv("SETTINGS_URL")); err != nil {
			log.Fatal(err)
		}
	}

	for key, body := range docs {
		if *dryRun {
			fmt.Printf("--- %s\n%s\n", key, body)
			continue
		}
		if err := bucket.Put(ctx, key, body); err != nil {
			log.Fatalf("%s: %v", key, err)
		}
		fmt.Println("wrote", key)
	}
}

func load(ctx context.Context, dec *util.Decrypter) ([]row, error) {
	dsn := fmt.Sprintf("postgres://%s@%s:%s/%s",
		url.UserPassword(util.MustEnv("DB_USER"), util.MustEnv("DB_PASSWORD")).String(),
		util.MustEnv("DB_HOST"), util.MustEnv("DB_PORT"), util.MustEnv("DB_DATABASE"))

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer pool.Close()

	rs, err := pool.Query(ctx, `SELECT application, user_id::text, key, value, meta1, meta2 FROM user_settings`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()

	var out []row
	for rs.Next() {
		var r row
		if err := rs.Scan(&r.app, &r.userID, &r.key, &r.value, &r.meta1, &r.meta2); err != nil {
			return nil, err
		}
		if encrypted[r.key] {
			plain, err := dec.Decrypt(r.value)
			if err != nil {
				return nil, fmt.Errorf("%s/%s/%s: decrypt: %w", r.app, r.userID, r.key, err)
			}
			r.value = strings.TrimSpace(plain)
		}
		out = append(out, r)
	}
	return out, rs.Err()
}

// convert groups rows into documents keyed by their bucket path.
func convert(rows []row, admin string) map[string][]byte {
	type pk struct{ app, user, pid string }

	appRows := map[string][]row{}    // admin-level, by application
	procRows := map[pk][]row{}       // per process
	states := map[pk]clients.State{} // process_state rows

	for _, r := range rows {
		switch {
		case r.meta1 == "" && r.userID == admin:
			appRows[r.app] = append(appRows[r.app], r)
		case r.meta1 != "" && r.key == "process_state":
			states[pk{r.app, r.userID, r.meta1}] = clients.State(r.value)
		case r.meta1 != "":
			k := pk{r.app, r.userID, r.meta1}
			procRows[k] = append(procRows[k], r)
		}
	}

	docs := map[string][]byte{}

	for app, rs := range appRows {
		switch app {
		case "pegasus":
			docs[doc.AppKey(app)] = marshal(pegasusApp(rs))
		case "betfair":
			docs[doc.AppKey(app)] = marshal(betfairAccount(rs))
		case "betmatic":
			docs[doc.AppKey(app)] = marshal(betmaticAccount(rs))
		case "davo":
			docs[doc.AppKey(app)] = marshal(davoApp(rs))
		}
	}

	for k, rs := range procRows {
		key := clients.ProcessKey{App: k.app, UserID: k.user, ProcessID: k.pid}
		switch k.app {
		case "pegasus":
			docs[doc.ProcessKey(key)] = marshal(pegasusProcess(rs))
		case "davo":
			docs[doc.ProcessKey(key)] = marshal(davoProcess(rs))
		default:
			continue
		}
		state := states[k]
		if state == "" {
			state = clients.StateStopped
		}
		docs[doc.StateKey(key)] = marshal(map[string]string{"state": string(state)})
	}

	return docs
}

// ---- per-app shapes; these mirror each app's settings.ParseProcess ----

func kv(rs []row) map[string]string {
	m := map[string]string{}
	for _, r := range rs {
		if r.meta2 == "" {
			m[r.key] = r.value
		}
	}
	return m
}

func pegasusApp(rs []row) map[string]any {
	m := kv(rs)
	return map[string]any{
		"feeds": map[string]bool{
			"triples": boolOr(m["triples_enabled"], true),
			"tpd":     boolOr(m["tpd_enabled"], false),
		},
		"triples": map[string]string{
			"endpoint": m["endpoint"], "region": m["region"], "access_key_id": m["access_kid"],
			"secret_access_key": m["secret_access_key"], "client_id": m["client_id"],
		},
		"tpd": map[string]string{"licence_key": m["tpd_licence_key"], "udp_port": m["tpd_udp_port"]},
	}
}

func betfairAccount(rs []row) map[string]string {
	m := kv(rs)
	return map[string]string{
		"username": m["betfair_username"], "password": m["betfair_password"],
		"app_key": m["betfair_app_key"], "cert": m["betfair_pem_key"],
	}
}

func betmaticAccount(rs []row) map[string]any {
	m := kv(rs)
	return map[string]any{
		"username": m["betmatic_username"], "password": m["betmatic_password"],
		"bot_id": m["bot_id"], "bookmakers": list(m["bookmakers"]),
	}
}

func davoApp(rs []row) map[string]any {
	m := kv(rs)
	return map[string]any{
		"telegram":      map[string]any{"bot_token": m["telegram_bot_token_key"], "scrape_channel_id": intOr(m["telegram_scrape_channel_id"])},
		"anthropic_key": m["anthropic_key"],
	}
}

func pegasusProcess(rs []row) map[string]any {
	m := kv(rs)

	scopes := map[string]map[string]any{}
	for _, r := range rs {
		if r.meta2 == "" {
			continue
		}
		s, ok := scopes[r.meta2]
		if !ok {
			s = map[string]any{
				"betmatic": map[string]any{}, "betfair": map[string]any{},
				"betmatic_delay_ms": int64(0), "betfair_delay_ms": int64(0),
			}
			scopes[r.meta2] = s
		}
		bm := s["betmatic"].(map[string]any)
		bf := s["betfair"].(map[string]any)

		switch r.key {
		case "betmatic_win_mbl":
			bm["win_mbl"] = boolOr(r.value, false)
		case "betmatic_win_stake":
			bm["win_stake"] = floatOr(r.value)
		case "betmatic_min_odds":
			bm["min_odds"] = floatOr(r.value)
		case "betmatic_max_odds":
			bm["max_odds"] = floatOr(r.value)
		case "betmatic_delay":
			s["betmatic_delay_ms"] = intOr(r.value)
		case "betfair_back_stake":
			bf["back_stake"] = floatOr(r.value)
		case "betfair_lay_stake":
			bf["lay_stake"] = floatOr(r.value)
		case "betfair_min_odds":
			bf["min_odds"] = floatOr(r.value)
		case "betfair_max_odds":
			bf["max_odds"] = floatOr(r.value)
		case "betfair_delay":
			s["betfair_delay_ms"] = intOr(r.value)
		}
	}

	return map[string]any{
		"betmatic": map[string]any{
			"username": m["betmatic_username"], "password": m["betmatic_password"],
			"bot_id": m["bot_id"], "bookmakers": list(m["bookmakers"]),
		},
		"betfair": map[string]string{
			"username": m["betfair_username"], "password": m["betfair_password"],
			"app_key": m["betfair_app_key"], "cert": m["betfair_pem_key"],
		},
		"scopes": scopes,
	}
}

func davoProcess(rs []row) map[string]any {
	m := kv(rs)
	return map[string]any{
		"betmatic": map[string]any{
			"username": m["betmatic_username"], "password": m["betmatic_password"],
			"bot_id": m["bot_id"], "bookmakers": list(m["bookmakers"]),
		},
		"min_odds":           floatOr(m["min_odds"]),
		"max_odds":           floatOr(m["max_odds"]),
		"target_liability":   floatOr(m["target_liability"]),
		"min_odds_threshold": floatOr(m["min_odds_under_threshold"]),
		"target_mbl":         boolOr(m["target_mbl"], false),
		"is_test":            boolOr(m["is_test"], false),
	}
}

// ---- value helpers ----

func marshal(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	return b
}

func list(v string) []string {
	out := []string{}
	for _, t := range strings.Split(v, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func boolOr(v string, def bool) bool {
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func floatOr(v string) float64 {
	f, _ := strconv.ParseFloat(v, 64)
	return f
}

func intOr(v string) int64 {
	i, _ := strconv.ParseInt(v, 10, 64)
	return i
}
