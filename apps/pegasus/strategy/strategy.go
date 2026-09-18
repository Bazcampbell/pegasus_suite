// strategy/strategy.go

package strategy

import (
	"fmt"

	"pegasus_suite/apps/pegasus/core"

	"pegasus_suite/betting/betmatic"
)

type Strategy interface {
	Name() string
	Code() string // 4 chars; the engine folds it into the bookmaker strategy reference

	Select(u core.Update, betfairDelay, betmaticDelay int64,
		getBetfairRace func(core.RaceRef) *core.BetfairRace) (core.Decision, error)
}

// returns the strategy that handles that provider + code
func For(provider core.Provider, code betmatic.RacingCode) (Strategy, error) {
	switch provider {
	case core.ProviderTPD:
		return NewTPDLeader(), nil

	case core.ProviderTripleS:
		switch code {
		case betmatic.THOROUGHBRED:
			return NewForwardProgressThoroughbred(), nil
		case betmatic.HARNESS:
			return NewForwardProgressHarness(), nil
		}
	}

	return nil, fmt.Errorf("no strategy for provider %q code %q", provider, code)
}
