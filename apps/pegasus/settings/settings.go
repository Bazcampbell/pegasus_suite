// pegasus/settings/settings.go
//
// Pegasus's settings documents. The kernel hands over raw JSON; these parsers
// decode and validate it. Nothing here touches storage.

package settings

import (
	"encoding/json"
	"fmt"

	"racing_wagering/clients"
	"racing_wagering/engine"
)

// ---- process document ----
//
//	{
//	  "betmatic": {"username": "", "password": "", "bot_id": "", "bookmakers": ["3", "7"]},
//	  "betfair":  {"username": "", "password": "", "app_key": "", "cert": ""},
//	  "scopes": {
//	    "US/THOROUGHBRED": {
//	      "betmatic": {"win_stake": 10, "win_mbl": false, "min_odds": 1.5, "max_odds": 12},
//	      "betfair":  {"back_stake": 0, "lay_stake": 0, "min_odds": 0, "max_odds": 0},
//	      "betmatic_delay_ms": 2000,
//	      "betfair_delay_ms": 2000
//	    }
//	  }
//	}

// ProcessSettings is one bookmaker account pairing. What it bets is Scopes,
// keyed by core.ScopeKey — a scope absent from the map, or present with nothing
// staked, is never bet.
type ProcessSettings struct {
	ID     string `json:"-"`
	UserID string `json:"-"`

	BetmaticCredentials engine.BetmaticCredentials `json:"betmatic"`
	BetfairCredentials  engine.BetfairCredentials  `json:"betfair"`

	Scopes map[string]ScopeSettings `json:"scopes"`
}

// ScopeSettings is the money for one country/code, plus how long after the off
// each provider is bet. Active, BetsBetmatic and BetsBetfair come from Stake.
type ScopeSettings struct {
	engine.Stake

	BetmaticDelay int64 `json:"betmatic_delay_ms"`
	BetfairDelay  int64 `json:"betfair_delay_ms"`
}

// Credentials is what the engine opens sessions from: nil for a provider the
// process has no account with.
func (s *ProcessSettings) Credentials() engine.Credentials {
	var c engine.Credentials
	if s.BetmaticCredentials.Username != "" {
		bm := s.BetmaticCredentials
		c.Betmatic = &bm
	}
	if s.BetfairCredentials.Username != "" {
		bf := s.BetfairCredentials
		c.Betfair = &bf
	}
	return c
}

func ParseProcess(key clients.ProcessKey, doc json.RawMessage) (*ProcessSettings, error) {
	if len(doc) == 0 {
		return nil, fmt.Errorf("process not found")
	}

	s := &ProcessSettings{ID: key.ProcessID, UserID: key.UserID}
	if err := json.Unmarshal(doc, s); err != nil {
		return nil, fmt.Errorf("process settings: %w", err)
	}
	if s.Scopes == nil {
		s.Scopes = map[string]ScopeSettings{}
	}

	if err := s.validate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *ProcessSettings) validate() error {
	bfValid := validateBetfair(s.BetfairCredentials)
	bmValid := validateBetmatic(s.BetmaticCredentials)

	if bfValid != nil && bmValid != nil {
		return fmt.Errorf("betfair or betmatic setup must be valid")
	}

	var active int
	for key, scope := range s.Scopes {
		if !scope.Active() {
			continue
		}
		active++

		if scope.BetsBetmatic() {
			if bmValid != nil {
				return fmt.Errorf("scope %s stakes betmatic but betmatic is not configured", key)
			}
			if scope.BetmaticDelay < 500 {
				return fmt.Errorf("scope %s betmatic delay must be at least 500ms", key)
			}
		}

		if scope.BetsBetfair() {
			if bfValid != nil {
				return fmt.Errorf("scope %s stakes betfair but betfair is not configured", key)
			}
			if scope.BetfairDelay < 500 {
				return fmt.Errorf("scope %s betfair delay must be at least 500ms", key)
			}
			// Betfair needs a floor to price against; without one the limit
			// price calculation has nothing to reject a collapsed book with.
			if scope.Betfair.MinOdds < 1 {
				return fmt.Errorf("scope %s betfair min odds must be set", key)
			}
		}
	}

	if active == 0 {
		return fmt.Errorf("at least one scope must have a stake set")
	}
	return nil
}

func validateBetmatic(b engine.BetmaticCredentials) error {
	if b.Username == "" || b.Password == "" {
		return fmt.Errorf("betmatic username and password required")
	}
	return nil
}

func validateBetfair(b engine.BetfairCredentials) error {
	if b.Username == "" || b.Password == "" || b.AppKey == "" || b.Cert == "" {
		return fmt.Errorf("betfair username, password, app key and cert required")
	}
	return nil
}

// ---- application document (settings/apps/pegasus.json) ----
//
//	{
//	  "feeds":   {"triples": true, "tpd": false},
//	  "triples": {"endpoint": "", "region": "", "access_key_id": "", "secret_access_key": "", "client_id": ""},
//	  "tpd":     {"licence_key": "", "udp_port": "4629"}
//	}

type AppSettings struct {
	Feeds   Feeds   `json:"feeds"`
	TripleS TripleS `json:"triples"`
	TPD     TPD     `json:"tpd"`
}

type Feeds struct {
	TripleS bool `json:"triples"`
	TPD     bool `json:"tpd"`
}

type TripleS struct {
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	ClientID        string `json:"client_id"`
}

type TPD struct {
	LicenceKey string `json:"licence_key"`
	UDPPort    string `json:"udp_port"`
}

// Gmax default. Their helpsheet says any port can be arranged, so this is only
// what we listen on when nothing is configured.
const defaultTPDPort = "4629"

// ParseApp decodes the application document over its defaults: Triple-S on,
// TPD off, the Gmax default port. A missing document is those defaults.
func ParseApp(doc json.RawMessage) (*AppSettings, error) {
	a := &AppSettings{
		Feeds: Feeds{TripleS: true},
		TPD:   TPD{UDPPort: defaultTPDPort},
	}
	if len(doc) == 0 {
		return a, nil
	}
	if err := json.Unmarshal(doc, a); err != nil {
		return nil, fmt.Errorf("pegasus settings: %w", err)
	}
	if a.TPD.UDPPort == "" {
		a.TPD.UDPPort = defaultTPDPort
	}
	return a, nil
}

func (t TPD) Validate() error {
	if t.LicenceKey == "" {
		return fmt.Errorf("tpd licence key not set")
	}
	return nil
}

// ParseAdminBetfair reads the admin exchange account used for race and price
// lookup, from the "betfair" settings scope.
func ParseAdminBetfair(doc json.RawMessage) (*engine.BetfairCredentials, error) {
	var c engine.BetfairCredentials
	if len(doc) > 0 {
		if err := json.Unmarshal(doc, &c); err != nil {
			return nil, fmt.Errorf("betfair admin: %w", err)
		}
	}
	if err := validateBetfair(c); err != nil {
		return nil, fmt.Errorf("betfair admin: %w", err)
	}
	return &c, nil
}
