package settings

import (
	"strings"
	"testing"

	"pegasus_suite/engine"
)

func validProcess() *ProcessSettings {
	return &ProcessSettings{
		BetmaticCredentials: engine.BetmaticCredentials{Username: "u", Password: "p"},
		Scopes: map[string]ScopeSettings{
			"US/THOROUGHBRED": {
				Stake:         engine.Stake{Betmatic: engine.BetmaticStake{WinStake: 10}},
				BetmaticDelay: 2000,
			},
		},
	}
}

func TestProcessSettingsValidate(t *testing.T) {
	if err := validProcess().Validate(); err != nil {
		t.Fatalf("valid document: %v", err)
	}

	cases := map[string]struct {
		edit func(*ProcessSettings)
		want string
	}{
		"no accounts": {
			func(s *ProcessSettings) { s.BetmaticCredentials = engine.BetmaticCredentials{} },
			"betfair or betmatic",
		},
		"nothing staked": {
			func(s *ProcessSettings) { s.Scopes = nil },
			"at least one scope",
		},
		"unknown scope": {
			func(s *ProcessSettings) {
				s.Scopes["US/CAMELS"] = s.Scopes["US/THOROUGHBRED"]
			},
			"unknown scope",
		},
		"delay too short": {
			func(s *ProcessSettings) {
				sc := s.Scopes["US/THOROUGHBRED"]
				sc.BetmaticDelay = 100
				s.Scopes["US/THOROUGHBRED"] = sc
			},
			"delay",
		},
		"betfair staked without an account": {
			func(s *ProcessSettings) {
				sc := s.Scopes["US/THOROUGHBRED"]
				sc.Betfair.BackStake = 5
				s.Scopes["US/THOROUGHBRED"] = sc
			},
			"betfair is not configured",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s := validProcess()
			c.edit(s)
			err := s.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestFeedSettingsValidate(t *testing.T) {
	if err := DefaultTPD().Validate(); err != nil {
		t.Fatalf("tpd defaults: %v", err)
	}

	tpd := DefaultTPD()
	tpd.Enabled = true
	if err := tpd.Validate(); err == nil {
		t.Fatalf("tpd on without a licence key should fail")
	}
	tpd.LicenceKey = "k"
	if err := tpd.Validate(); err != nil {
		t.Fatalf("tpd with a licence key: %v", err)
	}
	tpd.UDPPort = "not-a-port"
	if err := tpd.Validate(); err == nil {
		t.Fatalf("bad udp port should fail")
	}

	// Triple-S defaults on, so its defaults alone are not a valid document:
	// an enabled feed has to say where it connects.
	tripleS := DefaultTripleS()
	if err := tripleS.Validate(); err == nil {
		t.Fatalf("triple-s on with nothing set should fail")
	}
	tripleS.Enabled = false
	if err := tripleS.Validate(); err != nil {
		t.Fatalf("triple-s off may be left empty: %v", err)
	}
	*tripleS = TripleS{Enabled: true, Endpoint: "e", Region: "r", AccessKeyID: "a", SecretAccessKey: "s", ClientID: "c"}
	if err := tripleS.Validate(); err != nil {
		t.Fatalf("triple-s fully set: %v", err)
	}
}
