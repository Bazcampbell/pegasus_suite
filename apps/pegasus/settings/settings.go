// pegasus/settings/settings.go
//
// Pegasus's settings documents. Each type validates itself: the kernel runs
// Validate before a document is saved, and the parsers run it again when one
// is loaded. Nothing here touches storage.

package settings

import (
	"encoding/json"
	"fmt"
	"strconv"

	"pegasus_suite/apps/pegasus/core"
	"pegasus_suite/clients"
	"pegasus_suite/engine"
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

	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Validate checks what the document alone can tell. Whether a scope's feed is
// enabled depends on the running app, so that is checked when the process is
// built, not here.
func (s *ProcessSettings) Validate() error {
	bfValid := s.BetfairCredentials.Validate()
	bmValid := s.BetmaticCredentials.Validate()

	if bfValid != nil && bmValid != nil {
		return fmt.Errorf("betfair or betmatic setup must be valid")
	}

	var active int
	for key, scope := range s.Scopes {
		if !scope.Active() {
			continue
		}
		active++

		if _, _, ok := core.SplitScopeKey(key); !ok {
			return fmt.Errorf("unknown scope %q", key)
		}

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

// ---- feed documents (settings/apps/triples.json, settings/apps/tpd.json) ----
//
// One admin-level document per feed, each carrying its own on/off switch.
//
//	triples: {"enabled": true, "endpoint": "", "region": "", "access_key_id": "", "secret_access_key": "", "client_id": ""}
//	tpd:     {"enabled": false, "licence_key": "", "udp_port": "4629"}

const (
	TripleSDoc = "triples"
	TPDDoc     = "tpd"
)

type TripleS struct {
	Enabled         bool   `json:"enabled"`
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	ClientID        string `json:"client_id"`
}

type TPD struct {
	Enabled    bool   `json:"enabled"`
	LicenceKey string `json:"licence_key"`
	UDPPort    string `json:"udp_port"`
}

// Gmax default. Their helpsheet says any port can be arranged, so this is only
// what we listen on when nothing is configured.
const defaultTPDPort = "4629"

// Triple-S defaults on because that is what the runtime did before there was a
// choice; TPD defaults off so a deploy never starts a feed nobody configured.
func DefaultTripleS() *TripleS { return &TripleS{Enabled: true} }
func DefaultTPD() *TPD         { return &TPD{UDPPort: defaultTPDPort} }

// Validate holds an enabled feed to everything it connects with. A disabled
// feed may be left half filled in.
func (t *TripleS) Validate() error {
	if !t.Enabled {
		return nil
	}
	if t.Endpoint == "" || t.Region == "" || t.AccessKeyID == "" || t.SecretAccessKey == "" || t.ClientID == "" {
		return fmt.Errorf("triple-s endpoint, region, access key id, secret access key and client id required")
	}
	return nil
}

func (t *TPD) Validate() error {
	if t.UDPPort != "" {
		if p, err := strconv.Atoi(t.UDPPort); err != nil || p < 1 || p > 65535 {
			return fmt.Errorf("tpd udp port %q is not a port", t.UDPPort)
		}
	}
	if t.Enabled && t.LicenceKey == "" {
		return fmt.Errorf("tpd licence key not set")
	}
	return nil
}

// ParseTripleS and ParseTPD decode a feed document over its defaults; a
// missing document is the defaults. They do not Validate: a feed with bad
// settings fails on its own at start and is reported, rather than taking the
// app down.
func ParseTripleS(doc json.RawMessage) (*TripleS, error) {
	t := DefaultTripleS()
	if len(doc) > 0 {
		if err := json.Unmarshal(doc, t); err != nil {
			return nil, fmt.Errorf("triple-s settings: %w", err)
		}
	}
	return t, nil
}

func ParseTPD(doc json.RawMessage) (*TPD, error) {
	t := DefaultTPD()
	if len(doc) > 0 {
		if err := json.Unmarshal(doc, t); err != nil {
			return nil, fmt.Errorf("tpd settings: %w", err)
		}
	}
	if t.UDPPort == "" {
		t.UDPPort = defaultTPDPort
	}
	return t, nil
}

// ParseAdminBetfair reads the admin exchange account used for race and price
// lookup, from the shared "betfair" document.
func ParseAdminBetfair(doc json.RawMessage) (*engine.BetfairCredentials, error) {
	var c engine.BetfairCredentials
	if len(doc) > 0 {
		if err := json.Unmarshal(doc, &c); err != nil {
			return nil, fmt.Errorf("betfair admin: %w", err)
		}
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("betfair admin: %w", err)
	}
	return &c, nil
}
