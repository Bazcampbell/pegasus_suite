// davo/settings/settings.go
//
// Process document (settings/processes/davo/<user>/<pid>.json):
//
//	{
//	  "betmatic": {"username": "", "password": "", "bot_id": "", "bookmakers": ["3", "7"]},
//	  "target_liability": 100,
//	  "min_odds_threshold": 10,
//	  "is_test": false
//	}
//
// Admin document (settings/apps/davo.json):
//
//	{"telegram_bot_token": "", "scrape_channel_id": 0, "anthropic_api_key": ""}

package settings

import (
	"encoding/json"
	"errors"
	"fmt"

	"pegasus_suite/clients"
	"pegasus_suite/engine"
)

const AdminDoc = "davo"

type ProcessSettings struct {
	ID     string `json:"-"`
	UserID string `json:"-"`

	Betmatic engine.BetmaticCredentials `json:"betmatic"`

	// dollars a one-unit tip risks at its minimum odds
	TargetLiability float64 `json:"target_liability"`
	// how far below the rated price, in percent, a bet may still be taken
	MinOddsThreshold float64 `json:"min_odds_threshold"`
	// stakes $1 on every tip
	IsTest bool `json:"is_test"`
}

func (s *ProcessSettings) Validate() error {
	if err := s.Betmatic.Validate(); err != nil {
		return err
	}
	if s.TargetLiability <= 0 {
		return errors.New("target liability must be positive")
	}
	if s.MinOddsThreshold < 0 {
		return errors.New("min odds threshold cannot be negative")
	}
	return nil
}

func ParseProcess(key clients.ProcessKey, doc json.RawMessage) (*ProcessSettings, error) {
	s := &ProcessSettings{ID: key.ProcessID, UserID: key.UserID}
	if err := json.Unmarshal(doc, s); err != nil {
		return nil, fmt.Errorf("process settings: %w", err)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
}

type Admin struct {
	TelegramBotToken string `json:"telegram_bot_token"`
	ScrapeChannelID  int64  `json:"scrape_channel_id"`
	AnthropicAPIKey  string `json:"anthropic_api_key"`
}

func (a *Admin) Validate() error {
	if a.TelegramBotToken == "" || a.ScrapeChannelID == 0 {
		return errors.New("telegram bot token and scrape channel id required")
	}
	return nil
}

// ParseAdmin decodes and validates the admin document.
func ParseAdmin(doc json.RawMessage) (*Admin, error) {
	a := &Admin{}
	if len(doc) > 0 {
		if err := json.Unmarshal(doc, a); err != nil {
			return nil, fmt.Errorf("davo settings: %w", err)
		}
	}
	return a, a.Validate()
}
